package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/heptiolabs/healthcheck"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	heritageLabelKey           = "heritage"
	awsIAMControllerLabelValue = "kube-aws-iam-controller"
	secretPrefix               = "aws-iam-"
	roleARNKey                 = "role-arn"
	expireKey                  = "expire"
	credentialsFileKey         = "credentials"
	credentialsFileTemplate    = `[default]
aws_access_key_id = %s
aws_secret_access_key = %s
aws_session_token = %s
aws_expiration = %s
`
	credentialsProcessFileKey     = "credentials.process"
	credentialsProcessFileContent = `[default]
credential_process = cat /meta/aws-iam/credentials.json
`
	credentialsJSONFileKey = "credentials.json"
	healthEndpointAddress  = ":8080"
)

var (
	ownerLabels = map[string]string{heritageLabelKey: awsIAMControllerLabelValue}
)

// SecretsController is a controller which listens for pod events and updates
// secrets with AWS IAM roles as requested by pods.
type SecretsController struct {
	client         kubernetes.Interface
	interval       time.Duration
	refreshLimit   time.Duration
	creds          CredentialsGetter
	roleStore      *RoleStore
	namespace      string
	HealthReporter healthcheck.Handler
}

// ProcessCredentials defines the format expected from process credentials.
// https://docs.aws.amazon.com/cli/latest/topic/config-vars.html#sourcing-credentials-from-external-processes
type ProcessCredentials struct {
	Version         int       `json:"Version"`
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	SessionToken    string    `json:"SessionToken"`
	Expiration      time.Time `json:"Expiration"`
}

// NewSecretsController initializes a new SecretsController.
func NewSecretsController(client kubernetes.Interface, namespace string, interval, refreshLimit time.Duration, creds CredentialsGetter) *SecretsController {
	return &SecretsController{
		client:       client,
		interval:     interval,
		refreshLimit: refreshLimit,
		creds:        creds,
		roleStore:    NewRoleStore(),
		namespace:    namespace,
	}
}

// getCreds gets new credentials from the CredentialsGetter and converts them
// to a secret data map.
func (c *SecretsController) getCreds(role string) (map[string][]byte, error) {
	creds, err := c.creds.Get(role, 3600*time.Second)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return map[string][]byte{
		expireKey:                 []byte(creds.Expiration.Format(time.RFC3339)),
		credentialsFileKey:        []byte(credsFile),
		credentialsProcessFileKey: []byte(credentialsProcessFileContent),
		credentialsJSONFileKey:    processCredsData,
	}, nil
}

// Run runs the secret controller loop. This will refresh secrets with AWS IAM
// roles.
func (c *SecretsController) Run(ctx context.Context) {
	// Defining the liveness check
	var nextRefresh time.Time

	// If the controller hasn't refreshed credentials in a while, fail liveness
	c.HealthReporter.AddLivenessCheck("nextRefresh", func() error {
		if time.Since(nextRefresh) > 5*c.interval {
			return fmt.Errorf("nextRefresh too old")
		}
		return nil
	})

	nextRefresh = time.Now().Add(-c.interval)

	// Add the liveness endpoint at /healthz
	http.HandleFunc("/healthz", c.HealthReporter.LiveEndpoint)

	// Start the HTTP server
	http.ListenAndServe(healthEndpointAddress, nil)

	for {
		select {
		case <-time.After(time.Until(nextRefresh)):
			nextRefresh = time.Now().Add(c.interval)
			err := c.refresh(ctx)
			if err != nil {
				log.Error(err)
			}
		case <-ctx.Done():
			log.Info("Terminating main controller loop.")
			return
		}
	}
}

// refresh checks for soon to expire secrets and requests new credentials. It
// also looks for roles where secrets are missing and creates the secrets for
// the designated namespace.
func (c *SecretsController) refresh(ctx context.Context) error {
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(ownerLabels).AsSelector().String(),
	}

	secrets, err := c.client.CoreV1().Secrets(c.namespace).List(ctx, opts)
	if err != nil {
		return err
	}

	credsCache := map[string]map[string][]byte{}

	tmpSecretStore := NewRoleStore()

	for _, secret := range secrets.Items {
		// skip secrets owned by someone
		if len(secret.OwnerReferences) > 0 {
			continue
		}

		role := strings.TrimPrefix(secret.Name, secretPrefix)

		// store found secrets in a tmp store so we can lookup missing
		// role -> secret mappings later
		tmpSecretStore.Add(role, secret.Namespace, "")

		if !c.roleStore.Exists(role, secret.Namespace) {
			// TODO: mark for deletion first
			err := c.client.CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{})
			if err != nil {
				log.Errorf("Failed to delete secret %s/%s: %v", secret.Namespace, secret.Name, err)
				continue
			}

			log.WithFields(log.Fields{
				"action":    "delete",
				"role":      role,
				"secret":    secret.Name,
				"namespace": secret.Namespace,
			}).Info("Removing unused credentials")
			continue
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
			secret.Data, err = c.getCreds(role)
			if err != nil {
				log.Errorf("Failed to get credentials for role %s: %v", role, err)
				continue
			}

			// update secret with refreshed credentials
			_, err := c.client.CoreV1().Secrets(secret.Namespace).Update(ctx, &secret, metav1.UpdateOptions{})
			if err != nil {
				log.Errorf("Failed to update secret %s/%s: %v", secret.Namespace, secret.Name, err)
				continue
			}

			log.WithFields(log.Fields{
				"action":    "update",
				"role":      role,
				"secret":    secret.Name,
				"namespace": secret.Namespace,
				"expire":    string(secret.Data[expireKey]),
			}).Info()
		}

		if secret.Data != nil {
			credsCache[role] = secret.Data
		}
	}

	// create missing secrets
	c.roleStore.RLock()
	for role, namespaces := range c.roleStore.Store {
		creds, ok := credsCache[role]
		if !ok {
			creds, err = c.getCreds(role)
			if err != nil {
				log.Errorf("Failed to get credentials for role %s: %v", role, err)
				continue
			}
		}

		for ns := range namespaces {
			if !tmpSecretStore.Exists(role, ns) {
				// create secret
				name := secretPrefix + role
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: ns,
						Labels:    ownerLabels,
					},
					Data: creds,
				}

				_, err := c.client.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
				if err != nil {
					log.Errorf("Failed to create secret %s/%s: %v", ns, name, err)
					continue
				}
				log.WithFields(log.Fields{
					"action":    "create",
					"role":      role,
					"secret":    name,
					"namespace": ns,
					"expire":    string(creds[expireKey]),
				}).Info()
			}
		}
	}
	c.roleStore.RUnlock()

	return nil
}
