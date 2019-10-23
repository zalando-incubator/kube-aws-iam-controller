package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	av1 "github.com/zalando-incubator/kube-aws-iam-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/kube-aws-iam-controller/pkg/clientset"
	"github.com/zalando-incubator/kube-aws-iam-controller/pkg/recorder"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
)

const (
	awsIAMRoleGenerationKey = "awsiamrole-generation"
)

var (
	awsIAMRoleOwnerLabels = map[string]string{
		heritageLabelKey: awsIAMControllerLabelValue,
		"type":           "awsiamrole",
	}
)

// AWSIAMRoleController is a controller which lists AWSIAMRole resources and
// create/update matching secrets with AWS IAM role credentials.
type AWSIAMRoleController struct {
	client       clientset.Interface
	recorder     record.EventRecorder
	interval     time.Duration
	refreshLimit time.Duration
	creds        CredentialsGetter
	namespace    string
}

// NewSecretsController initializes a new AWSIAMRoleController.
func NewAWSIAMRoleController(client clientset.Interface, interval, refreshLimit time.Duration, creds CredentialsGetter, namespace string) *AWSIAMRoleController {
	return &AWSIAMRoleController{
		client:       client,
		recorder:     recorder.CreateEventRecorder(client),
		interval:     interval,
		refreshLimit: refreshLimit,
		creds:        creds,
		namespace:    namespace,
	}
}

// getCreds gets new credentials from the CredentialsGetter and converts them
// to a secret data map.
func (c *AWSIAMRoleController) getCreds(role string, sessionDuration time.Duration) (*Credentials, map[string][]byte, error) {
	creds, err := c.creds.Get(role, sessionDuration)
	if err != nil {
		return nil, nil, err
	}

	credsFile := fmt.Sprintf(
		credentialsFileTemplate,
		creds.AccessKeyID,
		creds.SecretAccessKey,
		creds.SessionToken,
		creds.Expiration.Format(time.RFC3339),
	)

	processCreds := ProcessCredentials{
		Version:         1,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Expiration:      creds.Expiration,
	}

	processCredsData, err := json.Marshal(&processCreds)
	if err != nil {
		return nil, nil, err
	}

	return creds, map[string][]byte{
		roleARNKey:                []byte(creds.RoleARN),
		expireKey:                 []byte(creds.Expiration.Format(time.RFC3339)),
		credentialsFileKey:        []byte(credsFile),
		credentialsProcessFileKey: []byte(credentialsProcessFileContent),
		credentialsJSONFileKey:    processCredsData,
	}, nil
}

// Run runs the secret controller loop. This will refresh secrets with AWS IAM
// roles.
func (c *AWSIAMRoleController) Run(ctx context.Context) {
	for {
		err := c.refresh()
		if err != nil {
			log.Error(err)
		}

		select {
		case <-time.After(c.interval):
		case <-ctx.Done():
			log.Info("Terminating AWSIAMRole controller loop.")
			return
		}
	}
}

// refresh checks for soon to expire secrets and requests new credentials. It
// also looks for AWSIAMRole resources where secrets are missing and creates
// the secrets for the designated namespace.
func (c *AWSIAMRoleController) refresh() error {
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(awsIAMRoleOwnerLabels).AsSelector().String(),
	}

	secrets, err := c.client.CoreV1().Secrets(c.namespace).List(opts)
	if err != nil {
		return err
	}

	awsIAMRoles, err := c.client.ZalandoV1().AWSIAMRoles(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	secretsMap := make(map[string]v1.Secret, len(secrets.Items))
	for _, secret := range secrets.Items {
		secretsMap[secret.Namespace+"/"+secret.Name] = secret
	}

	awsIAMRolesMap := make(map[string]av1.AWSIAMRole, len(awsIAMRoles.Items))
	for _, role := range awsIAMRoles.Items {
		awsIAMRolesMap[role.Namespace+"/"+role.Name] = role
	}

	orphanSecrets := make([]v1.Secret, 0, len(secrets.Items))
	for _, secret := range secrets.Items {
		awsIAMRole, ok := awsIAMRolesMap[secret.Namespace+"/"+secret.Name]
		if !ok || !isOwnedReference(awsIAMRole.TypeMeta, awsIAMRole.ObjectMeta, secret.ObjectMeta) {
			orphanSecrets = append(orphanSecrets, secret)
			continue
		}

		role := awsIAMRole.Spec.RoleReference
		roleSessionDuration := 3600 * time.Second
		if awsIAMRole.Spec.RoleSessionDuration > 0 {
			roleSessionDuration = time.Duration(awsIAMRole.Spec.RoleSessionDuration) * time.Second
		}

		// TODO: move to function
		refreshCreds := false

		expirary, ok := secret.Data[expireKey]
		if !ok {
			refreshCreds = true
		} else {
			expire, err := time.Parse(time.RFC3339, string(expirary))
			if err != nil {
				log.Debugf("Failed to parse expirary time %s: %v", expirary, err)
				refreshCreds = true
			} else if time.Now().UTC().Add(c.refreshLimit).After(expire) {
				refreshCreds = true
			}
		}

		if refreshCreds {
			var creds *Credentials
			creds, secret.Data, err = c.getCreds(role, roleSessionDuration)
			if err != nil {
				c.recorder.Event(&awsIAMRole,
					v1.EventTypeWarning,
					"GetCredentialsFailed",
					fmt.Sprintf("Failed to get credentials for role '%s': %v", role, err),
				)
				continue
			}

			// update secret labels
			secret.Labels = mergeLabels(awsIAMRole.Labels, awsIAMRoleOwnerLabels)
			secret.Data[awsIAMRoleGenerationKey] = []byte(fmt.Sprintf("%d", awsIAMRole.Generation))

			// update secret with refreshed credentials
			_, err := c.client.CoreV1().Secrets(secret.Namespace).Update(&secret)
			if err != nil {
				log.Errorf("Failed to update secret %s/%s: %v", secret.Namespace, secret.Name, err)
				continue
			}

			log.WithFields(log.Fields{
				"action":    "update",
				"role-arn":  creds.RoleARN,
				"secret":    secret.Name,
				"namespace": secret.Namespace,
				"expire":    creds.Expiration.String(),
				"type":      "awsiamrole",
			}).Info()
			c.recorder.Event(&awsIAMRole,
				v1.EventTypeNormal,
				"UpdateCredentials",
				fmt.Sprintf("Updated credentials for role '%s', expiry time: %s", creds.RoleARN, creds.Expiration.String()),
			)

			// update AWSIAMRole status
			expiryTime := metav1.NewTime(creds.Expiration)
			awsIAMRole.Status = av1.AWSIAMRoleStatus{
				ObservedGeneration: &awsIAMRole.Generation,
				RoleARN:            creds.RoleARN,
				Expiration:         &expiryTime,
			}

			_, err = c.client.ZalandoV1().AWSIAMRoles(awsIAMRole.Namespace).UpdateStatus(&awsIAMRole)
			if err != nil {
				log.Errorf("Failed to update status for AWSIAMRole %s/%s: %v", awsIAMRole.Namespace, awsIAMRole.Name, err)
				continue
			}
		}
	}

	// clean up orphaned secrets
	for _, secret := range orphanSecrets {
		err := c.client.CoreV1().Secrets(secret.Namespace).Delete(secret.Name, nil)
		if err != nil {
			log.Errorf("Failed to delete secret %s/%s: %v", secret.Namespace, secret.Name, err)
			continue
		}

		log.WithFields(log.Fields{
			"action":    "delete",
			"role-arn":  string(secret.Data[roleARNKey]),
			"secret":    secret.Name,
			"namespace": secret.Namespace,
		}).Info("Removing unused credentials")
		continue
	}

	// create secrets for new AWSIAMRoles without a secret
	for _, awsIAMRole := range awsIAMRoles.Items {
		role := awsIAMRole.Spec.RoleReference
		roleSessionDuration := 3600 * time.Second
		if awsIAMRole.Spec.RoleSessionDuration > 0 {
			roleSessionDuration = time.Duration(awsIAMRole.Spec.RoleSessionDuration) * time.Second
		}

		if secret, ok := secretsMap[awsIAMRole.Namespace+"/"+awsIAMRole.Name]; ok {
			// update secret if out of date
			generation, err := getGeneration(secret.Data)
			if err != nil {
				c.recorder.Event(&awsIAMRole,
					v1.EventTypeWarning,
					"ReadSecretFailed",
					fmt.Sprintf("Failed to parse AWSIAMRole generation from secret %s/%s: %v",
						secret.Namespace,
						secret.Name,
						err),
				)
				continue
			}

			if awsIAMRole.Generation != generation {
				var creds *Credentials
				creds, secret.Data, err = c.getCreds(role, roleSessionDuration)
				if err != nil {
					c.recorder.Event(&awsIAMRole,
						v1.EventTypeWarning,
						"GetCredentialsFailed",
						fmt.Sprintf("Failed to get credentials for role '%s': %v", role, err),
					)
					continue
				}

				// update secret labels
				secret.Labels = mergeLabels(awsIAMRole.Labels, awsIAMRoleOwnerLabels)
				secret.Data[awsIAMRoleGenerationKey] = []byte(fmt.Sprintf("%d", awsIAMRole.Generation))

				// update secret with refreshed credentials
				_, err := c.client.CoreV1().Secrets(secret.Namespace).Update(&secret)
				if err != nil {
					c.recorder.Event(&awsIAMRole,
						v1.EventTypeWarning,
						"UpdateSecretFailed",
						fmt.Sprintf("Failed to update secret %s/%s with credentials: %v", secret.Namespace, secret.Name, err),
					)
					continue
				}

				log.WithFields(log.Fields{
					"action":    "update",
					"role-arn":  creds.RoleARN,
					"secret":    secret.Name,
					"namespace": secret.Namespace,
					"expire":    creds.Expiration.String(),
					"type":      "awsiamrole",
				}).Info()
				c.recorder.Event(&awsIAMRole,
					v1.EventTypeNormal,
					"UpdateCredentials",
					fmt.Sprintf("Updated credentials for role '%s', expiry time: %s", creds.RoleARN, creds.Expiration.String()),
				)
			}

			// update AWSIAMRole status if not up to date
			if awsIAMRole.Status.ObservedGeneration == nil || *awsIAMRole.Status.ObservedGeneration != awsIAMRole.Generation {
				expiration, err := time.Parse(time.RFC3339, string(secret.Data[expireKey]))
				if err != nil {
					c.recorder.Event(&awsIAMRole,
						v1.EventTypeWarning,
						"ReadSecretFailed",
						fmt.Sprintf("Failed to parse expirary time '%s' from secret %s/%s: %v",
							string(secret.Data[expireKey]),
							secret.Namespace,
							secret.Name,
							err),
					)
					continue
				}
				expiryTime := metav1.NewTime(expiration)
				awsIAMRole.Status = av1.AWSIAMRoleStatus{
					ObservedGeneration: &awsIAMRole.Generation,
					RoleARN:            string(secret.Data[roleARNKey]),
					Expiration:         &expiryTime,
				}

				// update AWSIAMRole status
				_, err = c.client.ZalandoV1().AWSIAMRoles(awsIAMRole.Namespace).UpdateStatus(&awsIAMRole)
				if err != nil {
					log.Errorf("Failed to update status of AWSIAMRole %s/%s: %v", awsIAMRole.Namespace, awsIAMRole.Name, err)
					continue
				}
			}
			continue
		}

		creds, secretData, err := c.getCreds(role, roleSessionDuration)
		if err != nil {
			c.recorder.Event(&awsIAMRole,
				v1.EventTypeWarning,
				"GetCredentialsFailed",
				fmt.Sprintf("Failed to get credentials for role '%s': %v", role, err),
			)
			continue
		}

		secretData[awsIAMRoleGenerationKey] = []byte(fmt.Sprintf("%d", awsIAMRole.Generation))

		// create secret
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      awsIAMRole.Name,
				Namespace: awsIAMRole.Namespace,
				Labels:    mergeLabels(awsIAMRole.Labels, awsIAMRoleOwnerLabels),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: awsIAMRole.APIVersion,
						Kind:       awsIAMRole.Kind,
						Name:       awsIAMRole.Name,
						UID:        awsIAMRole.UID,
					},
				},
			},
			Data: secretData,
		}

		_, err = c.client.CoreV1().Secrets(awsIAMRole.Namespace).Create(secret)
		if err != nil {
			c.recorder.Event(&awsIAMRole,
				v1.EventTypeWarning,
				"CreateSecretFailed",
				fmt.Sprintf("Failed to create secret %s/%s with credentials: %v", awsIAMRole.Namespace, awsIAMRole.Name, err),
			)
			continue
		}
		log.WithFields(log.Fields{
			"action":    "create",
			"role-arn":  creds.RoleARN,
			"secret":    awsIAMRole.Name,
			"namespace": awsIAMRole.Namespace,
			"expire":    creds.Expiration.String(),
			"type":      "awsiamrole",
		}).Info()
		c.recorder.Event(&awsIAMRole,
			v1.EventTypeNormal,
			"CreateCredentials",
			fmt.Sprintf("Created credentials for role '%s', expiry time: %s", creds.RoleARN, creds.Expiration.String()),
		)

		// update AWSIAMRole status
		expiryTime := metav1.NewTime(creds.Expiration)
		awsIAMRole.Status = av1.AWSIAMRoleStatus{
			ObservedGeneration: &awsIAMRole.Generation,
			RoleARN:            creds.RoleARN,
			Expiration:         &expiryTime,
		}

		_, err = c.client.ZalandoV1().AWSIAMRoles(awsIAMRole.Namespace).UpdateStatus(&awsIAMRole)
		if err != nil {
			log.Errorf("Failed to update status of AWSIAMRole %s/%s: %v", awsIAMRole.Namespace, awsIAMRole.Name, err)
			continue
		}
	}

	return nil
}

// isOwnedReference returns true of the dependent object is owned by the owner
// object.
func isOwnedReference(ownerTypeMeta metav1.TypeMeta, ownerObjectMeta, dependent metav1.ObjectMeta) bool {
	for _, ref := range dependent.OwnerReferences {
		if ref.APIVersion == ownerTypeMeta.APIVersion &&
			ref.Kind == ownerTypeMeta.Kind &&
			ref.UID == ownerObjectMeta.UID &&
			ref.Name == ownerObjectMeta.Name {
			return true
		}
	}
	return false
}

func mergeLabels(base, additional map[string]string) map[string]string {
	labels := make(map[string]string, len(base)+len(additional))
	for k, v := range base {
		labels[k] = v
	}

	for k, v := range additional {
		labels[k] = v
	}
	return labels
}

func getGeneration(secretData map[string][]byte) (int64, error) {
	generation := int64(0)
	gen, ok := secretData[awsIAMRoleGenerationKey]
	if ok {
		i, err := strconv.ParseInt(string(gen), 10, 64)
		if err != nil {
			return 0, err
		}
		generation = i
	}
	return generation, nil
}
