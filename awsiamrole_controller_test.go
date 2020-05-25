package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	av1 "github.com/zalando-incubator/kube-aws-iam-controller/pkg/apis/zalando.org/v1"
	fakeAWS "github.com/zalando-incubator/kube-aws-iam-controller/pkg/client/clientset/versioned/fake"
	"github.com/zalando-incubator/kube-aws-iam-controller/pkg/clientset"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	fakeKube "k8s.io/client-go/kubernetes/fake"
)

func TestIsOwnedReference(t *testing.T) {
	owner := av1.AWSIAMRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "zalando.org/v1",
			Kind:       "AWSIAMRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test1",
			UID:  types.UID("1234"),
		},
	}

	dependent := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: owner.Name,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: owner.APIVersion,
					Kind:       owner.Kind,
					Name:       owner.Name,
					UID:        owner.UID,
				},
			},
		},
	}

	require.True(t, isOwnedReference(owner.TypeMeta, owner.ObjectMeta, dependent.ObjectMeta))

	dependent.OwnerReferences[0].UID = types.UID("4321")
	require.False(t, isOwnedReference(owner.TypeMeta, owner.ObjectMeta, dependent.ObjectMeta))
}

func TestRefreshAWSIAMRole(tt *testing.T) {
	for _, tc := range []struct {
		msg             string
		secrets         []v1.Secret
		awsIAMRoles     []av1.AWSIAMRole
		credsGetter     CredentialsGetter
		expectedSecrets []v1.Secret
	}{
		{
			msg: "",
			credsGetter: &mockCredsGetter{
				creds: &Credentials{
					RoleARN: "arn",
				},
			},
			secrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "non-expired",
						Labels: awsIAMRoleOwnerLabels,
						OwnerReferences: []metav1.OwnerReference{
							{
								Name: "non-expired",
							},
						},
					},
					Data: map[string][]byte{
						expireKey:               []byte(time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)),
						roleARNKey:              []byte("arn"),
						awsIAMRoleGenerationKey: []byte("1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "expired",
						Labels: awsIAMRoleOwnerLabels,
						OwnerReferences: []metav1.OwnerReference{
							{
								Name: "expired",
							},
						},
					},
					Data: map[string][]byte{
						expireKey:  []byte(time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339)),
						roleARNKey: []byte("arn"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "orphan",
						Labels: awsIAMRoleOwnerLabels,
					},
				},
			},
			awsIAMRoles: []av1.AWSIAMRole{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "non-expired",
						Generation: 2,
					},
					Spec: av1.AWSIAMRoleSpec{
						RoleReference: "arn",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "expired",
					},
					Spec: av1.AWSIAMRoleSpec{
						RoleReference: "arn",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "new",
					},
					Spec: av1.AWSIAMRoleSpec{
						RoleReference: "arn",
					},
				},
			},
			expectedSecrets: []v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "non-expired",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "expired",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "new",
					},
				},
			},
		},
	} {
		tt.Run(tc.msg, func(t *testing.T) {
			awsKubeClient := fakeAWS.NewSimpleClientset()
			kubeClient := fakeKube.NewSimpleClientset()
			client := clientset.NewClientset(kubeClient, awsKubeClient)

			for _, role := range tc.awsIAMRoles {
				_, err := client.ZalandoV1().AWSIAMRoles("default").Create(context.TODO(), &role, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			for _, secret := range tc.secrets {
				_, err := client.CoreV1().Secrets("default").Create(context.TODO(), &secret, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			controller := NewAWSIAMRoleController(client, 0, 15*time.Minute, tc.credsGetter, "default")
			err := controller.refresh(context.TODO())
			require.NoError(t, err)

			secrets, err := client.CoreV1().Secrets("default").List(context.TODO(), metav1.ListOptions{})
			require.NoError(t, err)
			require.Len(t, secrets.Items, len(tc.expectedSecrets))
		})
	}
}
