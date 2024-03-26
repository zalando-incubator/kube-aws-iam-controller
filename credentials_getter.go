package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

const (
	roleARNSuffix          = ":role"
	roleSessionNameMaxSize = 64
)

// CredentialsGetter can get credentials.
type CredentialsGetter interface {
	Get(role string, sessionDuration time.Duration) (*Credentials, error)
}

// Credentials defines fetched credentials including expiration time.
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
	svc               stsiface.STSAPI
	baseRoleARN       string
	baseRoleARNPrefix string
}

// NewSTSCredentialsGetter initializes a new STS based credentials fetcher.
func NewSTSCredentialsGetter(sess *session.Session, baseRoleARN, baseRoleARNPrefix string, config *aws.Config) *STSCredentialsGetter {
	return &STSCredentialsGetter{
		svc:               sts.New(sess, config),
		baseRoleARN:       baseRoleARN,
		baseRoleARNPrefix: baseRoleARNPrefix,
	}
}

// Get gets new credentials for the specified role. The credentials are fetched
// via STS.
func (c *STSCredentialsGetter) Get(role string, sessionDuration time.Duration) (*Credentials, error) {
	roleARN := c.baseRoleARN + role
	if strings.HasPrefix(role, c.baseRoleARNPrefix) {
		roleARN = role
	}

	roleSessionName, err := normalizeRoleARN(roleARN, c.baseRoleARNPrefix)
	if err != nil {
		return nil, err
	}

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
func normalizeRoleARN(roleARN, roleARNPrefix string) (string, error) {
	parts := strings.Split(roleARN, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid roleARN: %s", roleARN)
	}

	remainingChars := roleSessionNameMaxSize

	accountID := strings.TrimPrefix(parts[0], roleARNPrefix)
	accountID = strings.TrimSuffix(accountID, roleARNSuffix)

	remainingChars -= len(accountID)

	return accountID + normalizePath(parts[1:], remainingChars), nil
}

// normalizePath normalizes the path levels into a roleSession valid string.
// The last level always gets as many chars as possible leaving only a minimum
// of one char for each of the other levels.
// e.g. given the levels: ["aaaaa", "bbbbb", "ccccccc"], and remaining "12" it
// would be reduced to the string: ".a.b.ccccccc"
func normalizePath(levels []string, remaining int) string {
	if len(levels) == 0 {
		return ""
	}

	last := levels[len(levels)-1]
	last = strings.Replace(last, ":", "_", -1)
	otherLevels := len(levels[:len(levels)-1])
	maxName := remaining - (otherLevels * 2) - 1

	if len(last) > maxName {
		last = last[:maxName]
	}
	return normalizePath(levels[:len(levels)-1], remaining-len(last)-1) + "." + last
}

// GetPrefixFromARN returns the prefix from an AWS ARN as a string.
// e.g. given the role: "arn:aws:iam::012345678910:role/role-name" it would
// return the string: "arn:aws:iam::"
func GetPrefixFromARN(inputARN string) (string, error) {
	arn, err := arn.Parse(inputARN)
	if err != nil {
		return "", fmt.Errorf("error parsing ARN (%s): %s", inputARN, err)
	}
	return fmt.Sprintf("arn:%s:iam::", arn.Partition), nil
}
