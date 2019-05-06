package main

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	"github.com/stretchr/testify/require"
)

type mockSTSAPI struct {
	stsiface.STSAPI
	err            error
	assumeRoleResp *sts.AssumeRoleOutput
}

func (sts *mockSTSAPI) AssumeRole(*sts.AssumeRoleInput) (*sts.AssumeRoleOutput, error) {
	if sts.err != nil {
		return nil, sts.err
	}
	return sts.assumeRoleResp, nil
}

func TestGet(t *testing.T) {
	sess := session.New(&aws.Config{Region: aws.String("region")})
	getter := NewSTSCredentialsGetter(sess, "", 1 * time.Hour)
	getter.svc = &mockSTSAPI{
		err: nil,
		assumeRoleResp: &sts.AssumeRoleOutput{
			Credentials: &sts.Credentials{
				AccessKeyId:     aws.String("access_key_id"),
				SecretAccessKey: aws.String("secret_access_key"),
				SessionToken:    aws.String("session_token"),
				Expiration:      &time.Time{},
			},
		},
	}

	creds, err := getter.Get("role")
	require.NoError(t, err)
	require.Equal(t, "access_key_id", creds.AccessKeyID)
	require.Equal(t, "secret_access_key", creds.SecretAccessKey)
	require.Equal(t, "session_token", creds.SessionToken)
	require.Equal(t, &time.Time{}, creds.Expiration)

	getter.svc = &mockSTSAPI{
		err: errors.New("failed"),
	}
	_, err = getter.Get("role")
	require.Error(t, err)
}

// func TestGetBaseRoleARN(t *testing.T) {
// 	sess := &session.Session{}
// 	baseRole, err := GetBaseRoleARN(sess)
// }
