package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type mockCredsGetter struct {
	err   error
	creds *Credentials
}

func (g *mockCredsGetter) Get(role string, sessionDuration time.Duration) (*Credentials, error) {
	if g.err != nil {
		return nil, g.err
	}
	return g.creds, nil
}

type roleTuple struct {
	Role      string
	Namespace string
	Pod       string
}

func TestRefresh(tt *testing.T) {
	timeFuture := time.Now().Add(time.Hour)
	timePast := time.Now().Add(-time.Hour)

	for _, ti := range []struct {
		msg             string
		secrets         []v1.Secret
		roles           []roleTuple
		expectedSecrets int
	}{
		{
			msg: "test removing secret for role no longer existing",
			secrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretPrefix + "role1",
						Namespace: "default",
						Labels:    ownerLabels,
					},
					Data: map[string][]byte{
						expireKey: []byte(timePast.Format(time.RFC3339)),
					},
				},
			},
			expectedSecrets: 0,
		},
		{
			msg: "test doing nothing for non-expired credentials",
			secrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretPrefix + "role1",
						Namespace: "default",
						Labels:    ownerLabels,
					},
					Data: map[string][]byte{
						expireKey: []byte(timeFuture.Format(time.RFC3339)),
					},
				},
			},
			roles: []roleTuple{
				{
					Role:      "role1",
					Namespace: "default",
					Pod:       "pod1",
				},
			},
			expectedSecrets: 1,
		},
		{
			msg: "test adding secret for new role",
			roles: []roleTuple{
				{
					Role:      "role1",
					Namespace: "default",
					Pod:       "pod1",
				},
			},
			expectedSecrets: 1,
		},
		{
			msg: "test refreshing creds",
			secrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretPrefix + "role1",
						Namespace: "default",
						Labels:    ownerLabels,
					},
					Data: map[string][]byte{
						expireKey: []byte(timePast.Format(time.RFC3339)),
					},
				},
			},
			roles: []roleTuple{
				{
					Role:      "role1",
					Namespace: "default",
					Pod:       "pod1",
				},
			},
			expectedSecrets: 1,
		},
		{
			msg: "test refreshing creds, when expiry is invalid",
			secrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretPrefix + "role1",
						Namespace: "default",
						Labels:    ownerLabels,
					},
					Data: map[string][]byte{
						expireKey: []byte(""),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretPrefix + "role2",
						Namespace: "default",
						Labels:    ownerLabels,
					},
					Data: map[string][]byte{},
				},
			},
			roles: []roleTuple{
				{
					Role:      "role1",
					Namespace: "default",
					Pod:       "pod1",
				},
				{
					Role:      "role2",
					Namespace: "default",
					Pod:       "pod1",
				},
			},
			expectedSecrets: 2,
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			controller := NewSecretsController(
				fake.NewSimpleClientset(),
				v1.NamespaceAll,
				time.Second,
				time.Second,
				&mockCredsGetter{
					creds: &Credentials{
						AccessKeyID:     "access_key_id",
						SecretAccessKey: "secret_access_key",
						SessionToken:    "session_token",
						Expiration:      timeFuture,
					},
				},
			)

			// setup secrets
			for _, secret := range ti.secrets {
				_, err := controller.client.CoreV1().Secrets("default").Create(context.TODO(), &secret, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			// setup roles
			for _, role := range ti.roles {
				controller.roleStore.Add(role.Role, role.Namespace, role.Pod)
			}

			err := controller.refresh(context.TODO())
			require.NoError(t, err)

			secrets, err := controller.client.CoreV1().Secrets("default").List(context.TODO(), metav1.ListOptions{})
			require.NoError(t, err)
			require.Len(t, secrets.Items, ti.expectedSecrets)
		})
	}
}
