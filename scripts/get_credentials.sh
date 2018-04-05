#!/bin/bash

role_arn="$1"
assume_role="$(aws sts assume-role --role-arn "$role_arn" --role-session-name tmp-session)"

aws_access_key_id="$(echo -n "$assume_role" | jq -r '.Credentials.AccessKeyId')"
aws_secret_access_key="$(echo -n "$assume_role" | jq -r '.Credentials.SecretAccessKey')"
aws_session_token="$(echo -n "$assume_role" | jq -r '.Credentials.SessionToken')"
aws_expiration="$(echo -n "$assume_role" | jq -r '.Credentials.Expiration')"

printf "[default]\naws_access_key_id = ${aws_access_key_id}\naws_secret_access_key = $aws_secret_access_key\naws_session_token = $aws_session_token\naws_expiration = $aws_expiration\n"
