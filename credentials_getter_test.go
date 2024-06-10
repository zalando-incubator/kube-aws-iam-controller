package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/require"
)

type mockSTSAPI struct {
	err            error
	assumeRoleResp *sts.AssumeRoleOutput
}

func (sts *mockSTSAPI) AssumeRole(_ context.Context, _ *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	if sts.err != nil {
		return nil, sts.err
	}
	return sts.assumeRoleResp, nil
}

func TestGet(t *testing.T) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		require.NoError(t, err)
	}
	cfg.Region = "region"
	getter := NewSTSCredentialsGetter(cfg, "", "")
	getter.svc = &mockSTSAPI{
		err: nil,
		assumeRoleResp: &sts.AssumeRoleOutput{
			Credentials: &types.Credentials{
				AccessKeyId:     aws.String("access_key_id"),
				SecretAccessKey: aws.String("secret_access_key"),
				SessionToken:    aws.String("session_token"),
				Expiration:      &time.Time{},
			},
		},
	}

	roleARN := "arn:aws:iam::012345678910:role/role-name"
	creds, err := getter.Get(context.Background(), roleARN, 3600*time.Second)
	require.NoError(t, err)
	require.Equal(t, "access_key_id", creds.AccessKeyID)
	require.Equal(t, "secret_access_key", creds.SecretAccessKey)
	require.Equal(t, "session_token", creds.SessionToken)
	require.Equal(t, time.Time{}, creds.Expiration)

	getter.svc = &mockSTSAPI{
		err: errors.New("failed"),
	}
	roleARNPrefix, err := GetPrefixFromARN(roleARN)
	require.NoError(t, err)
	_, err = getter.Get(context.Background(), roleARNPrefix+"role", 3600*time.Second)
	require.Error(t, err)
}

// func TestGetBaseRoleARN(t *testing.T) {
// 	sess := &session.Session{}
// 	baseRole, err := GetBaseRoleARN(sess)
// }

func TestGetPrefixFromARN(tt *testing.T) {
	for _, tc := range []struct {
		msg            string
		roleARN        string
		expectedPrefix string
	}{
		{
			msg:            "commercial AWS partition",
			roleARN:        "arn:aws:iam::012345678910:role/com-cloud",
			expectedPrefix: "arn:aws:iam::",
		},
		{
			msg:            "us gov AWS partition",
			roleARN:        "arn:aws-us-gov:iam::012345678910:role/gov-cloud",
			expectedPrefix: "arn:aws-us-gov:iam::",
		},
		{
			msg:            "china AWS partition",
			roleARN:        "arn:aws-cn:iam::012345678910:role/cn-cloud",
			expectedPrefix: "arn:aws-cn:iam::",
		},
	} {
		tt.Run(tc.msg, func(t *testing.T) {
			normalized, err := GetPrefixFromARN(tc.roleARN)
			require.NoError(t, err)
			require.Equal(t, tc.expectedPrefix, normalized)
		})
	}
}

func TestNormalizeRoleARN(tt *testing.T) {
	for _, tc := range []struct {
		msg         string
		roleARN     string
		expectedARN string
	}{
		{
			msg:         "simple role",
			roleARN:     "arn:aws:iam::012345678910:role/role-name",
			expectedARN: "012345678910.role-name",
		},
		{
			msg:         "truncate long role names",
			roleARN:     "arn:aws:iam::012345678910:role/role-name-very-very-very-very-very-very-very-very-long",
			expectedARN: "012345678910.role-name-very-very-very-very-very-very-very-very-l",
		},
		{
			msg:         "role name with path",
			roleARN:     "arn:aws:iam::012345678910:role/path-name/role-name",
			expectedARN: "012345678910.path-name.role-name",
		},
		{
			msg:         "us gov partition role name with path",
			roleARN:     "arn:aws-us-gov:iam::012345678910:role/path-name/role-name",
			expectedARN: "012345678910.path-name.role-name",
		},
		{
			msg:         "china partition role name with path",
			roleARN:     "arn:aws-cn:iam::012345678910:role/path-name/role-name",
			expectedARN: "012345678910.path-name.role-name",
		},
		{
			msg:         "truncate path for long role names",
			roleARN:     "arn:aws:iam::012345678910:role/aaaaa/bbbbb/ccccccccccccccccccccccccccccccccccccc-role-name",
			expectedARN: "012345678910.a.b.ccccccccccccccccccccccccccccccccccccc-role-name",
		},
	} {
		tt.Run(tc.msg, func(t *testing.T) {
			roleARNPrefix, err := GetPrefixFromARN(tc.roleARN)
			require.NoError(t, err)
			normalized, err := normalizeRoleARN(tc.roleARN, roleARNPrefix)
			require.NoError(t, err)
			require.Equal(t, tc.expectedARN, normalized)
		})
	}
}
