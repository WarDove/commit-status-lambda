# Description
This lambda written in Golang interacts with "Commit Status REST API" to update pipeline status visually in Github.

## Required IAM policies for lambda

```json
  {
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "VisualEditor0",
      "Effect": "Allow",
      "Action": [
        "codepipeline:ListActionExecutions",
        "codepipeline:GetPipelineExecution"
      ],
      "Resource": "*"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameters"
      ],
      "Resource": "arn:aws:ssm:region:account_id:parameter/parameter_name"
    }
  ]
}
```
Note: default labmda policies for logging are also needed

## SNS Topic policy

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "codestar-notifications.amazonaws.com"
      },
      "Action": "SNS:Publish",
      "Resource": "arn:aws:sns:eu-west-1:166733594871:githubCommitNotifier"
    }
  ]
}
```
