package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	heritageLabelKey           = "heritage"
	awsIAMControllerLabelValue = "kube-aws-iam-controller"
	secretPrefix               = "aws-iam-"
	expireKey                  = "expire"
	credentialsFileKey         = "credentials"
	credentialsFileTemplate    = `[default]
aws_access_key_id = %s
aws_secret_access_key = %s
aws_session_token = %s
aws_expiration = %s
`
)

var (
	ownerLabels = map[string]string{heritageLabelKey: awsIAMControllerLabelValue}
)

// SecretsController is a controller which listens for pod events and updates
// secrets with AWS IAM roles as requested by pods.
type SecretsController struct {
	client       kubernetes.Interface
	interval     time.Duration
	refreshLimit time.Duration
	creds        CredentialsGetter
	roleStore    *RoleStore
	podEvents    <-chan *PodEvent
}

// NewSecretsController initializes a new SecretsController.
func NewSecretsController(client kubernetes.Interface, interval, refreshLimit time.Duration, creds CredentialsGetter, podEvents <-chan *PodEvent) *SecretsController {
	return &SecretsController{
		client:       client,
		interval:     interval,
		refreshLimit: refreshLimit,
		creds:        creds,
		roleStore:    NewRoleStore(),
		podEvents:    podEvents,
	}
}

// getCreds gets new credentials from the CredentialsGetter and converts them
// to a secret data map.
func (c *SecretsController) getCreds(role string) (map[string][]byte, error) {
	creds, err := c.creds.Get(role)
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

	return map[string][]byte{
		expireKey:          []byte(creds.Expiration.Format(time.RFC3339)),
		credentialsFileKey: []byte(credsFile),
	}, nil
}

// Run runs the secret controller loop. This will refresh secrets with AWS IAM
// roles.
func (c *SecretsController) Run(stopCh <-chan struct{}) {
	go c.watchPods()

	for {
		err := c.refresh()
		if err != nil {
			log.Error(err)
		}

		select {
		case <-time.After(c.interval):
		case <-stopCh:
			log.Info("Terminating main controller loop.")
			return
		}
	}
}

// watchPods listens for pod events on the podEvents queue and updates the
// roleStore accordingly.
func (c *SecretsController) watchPods() {
	for {
		select {
		case event := <-c.podEvents:
			if event.Deletion {
				c.roleStore.Remove(event.Role, event.Namespace, event.Name)
			} else {
				c.roleStore.Add(event.Role, event.Namespace, event.Name)
			}
			// TODO:
			// case <-stopChan:
			// 	return
		}
	}
}

// refresh checks for soon to expire secrets and requests new credentials. It
// also looks for roles where secrets are missing and creates the secrets for
// the designated namespace.
func (c *SecretsController) refresh() error {
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(ownerLabels).AsSelector().String(),
	}

	secrets, err := c.client.CoreV1().Secrets(v1.NamespaceAll).List(opts)
	if err != nil {
		return err
	}

	credsCache := map[string]map[string][]byte{}

	tmpSecretStore := NewRoleStore()

	for _, secret := range secrets.Items {
		role := strings.TrimPrefix(secret.Name, secretPrefix)

		// store found secrets in a tmp store so we can lookup missing
		// role -> secret mappings later
		tmpSecretStore.Add(role, secret.Namespace, "")

		if !c.roleStore.Exists(role, secret.Namespace) {
			// TODO: mark for deletion first
			err := c.client.CoreV1().Secrets(secret.Namespace).Delete(secret.Name, nil)
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
			_, err := c.client.CoreV1().Secrets(secret.Namespace).Update(&secret)
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

				_, err := c.client.CoreV1().Secrets(ns).Create(secret)
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
