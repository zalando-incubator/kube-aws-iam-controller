package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

const (
	arnPrefix = "arn:aws:iam::"
)

// CredentialsGetter can get credentials.
type CredentialsGetter interface {
	Get(role string, sessionDuration time.Duration) (*Credentials, error)
}

// Credentials defines fecthed credentials including expiration time.
type Credentials struct {
	RoleARN         string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

// STSCredentialsGetter is a credentials getter for getting credentials from
// STS.
type STSCredentialsGetter struct {
	svc         stsiface.STSAPI
	baseRoleARN string
}

// NewSTSCredentialsGetter initializes a new STS based credentials fetcher.
func NewSTSCredentialsGetter(sess *session.Session, baseRoleARN string) *STSCredentialsGetter {
	return &STSCredentialsGetter{
		svc:         sts.New(sess),
		baseRoleARN: baseRoleARN,
	}
}

// Get gets new credentials for the specified role. The credentials are fetched
// via STS.
func (c *STSCredentialsGetter) Get(role string, sessionDuration time.Duration) (*Credentials, error) {
	roleARN := c.baseRoleARN + role
	if strings.HasPrefix(role, arnPrefix) {
		roleARN = role
	}
	roleSessionName := normalizeRoleARN(roleARN) + "-session"

	params := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(roleSessionName),
		DurationSeconds: aws.Int64(int64(sessionDuration.Seconds())),
	}

	resp, err := c.svc.AssumeRole(params)
	if err != nil {
		return nil, err
	}

	return &Credentials{
		RoleARN:         roleARN,
		AccessKeyID:     aws.StringValue(resp.Credentials.AccessKeyId),
		SecretAccessKey: aws.StringValue(resp.Credentials.SecretAccessKey),
		SessionToken:    aws.StringValue(resp.Credentials.SessionToken),
		Expiration:      aws.TimeValue(resp.Credentials.Expiration),
	}, nil
}

// GetBaseRoleARN gets base role ARN from EC2 metadata service.
func GetBaseRoleARN(sess *session.Session) (string, error) {
	metadata := ec2metadata.New(sess)

	iamInfo, err := metadata.IAMInfo()
	if err != nil {
		return "", err
	}

	arn := strings.Replace(iamInfo.InstanceProfileArn, "instance-profile", "role", 1)
	baseRoleARN := strings.Split(arn, "/")
	if len(baseRoleARN) < 2 {
		return "", fmt.Errorf("failed to determine BaseRoleARN")
	}

	return fmt.Sprintf("%s/", baseRoleARN[0]), nil
}

// normalizeRoleARN normalizes a role ARN by substituting special characters
// with characters allowed for a RoleSessionName according to:
// https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
func normalizeRoleARN(roleARN string) string {
	roleARN = strings.Replace(roleARN, ":", "_", -1)
	roleARN = strings.Replace(roleARN, "/", ".", -1)
	return roleARN
}
