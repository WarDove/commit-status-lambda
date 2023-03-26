package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/service/ssm"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/codepipeline"
)

var (
	cp = codepipeline.New(session.Must(session.NewSession()))
)

type snsMessage struct {
	Account             string    `json:"account"`
	DetailType          string    `json:"detailType"`
	Region              string    `json:"region"`
	Source              string    `json:"source"`
	Time                time.Time `json:"time"`
	NotificationRuleArn string    `json:"notificationRuleArn"`
	Detail              struct {
		Pipeline         string `json:"pipeline"`
		ExecutionID      string `json:"execution-id"`
		ExecutionTrigger struct {
			TriggerType   string `json:"trigger-type"`
			TriggerDetail string `json:"trigger-detail"`
		} `json:"execution-trigger"`
		State string `json:"state"`
	} `json:"detail"`
	Resources            []string `json:"resources"`
	AdditionalAttributes struct {
	} `json:"additionalAttributes"`
}

func getParam(name string) (value string, err error) {

	ssmSession := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	ssmSvc := ssm.New(ssmSession)

	param, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", err
	}

	return *param.Parameter.Value, nil
}

func handler(ctx context.Context, snsEvent events.SNSEvent) error {
	ghTokenArn := os.Getenv("GITHUB_TOKEN_SECRET_ARN")

	for _, snsRecord := range snsEvent.Records {
		snsMsg := snsRecord.SNS.Message
		var message snsMessage

		if err := json.Unmarshal([]byte(snsMsg), &message); err != nil {
			return fmt.Errorf("failed to unmarshal SNS message: %v", err)
		}

		if message.Detail.State == "" || message.Detail.Pipeline == "" || message.Detail.ExecutionID == "" {
			return fmt.Errorf("missing required data in SNS message")
		}

		res, err := cp.GetPipelineExecution(&codepipeline.GetPipelineExecutionInput{
			PipelineName:        aws.String(message.Detail.Pipeline),
			PipelineExecutionId: aws.String(message.Detail.ExecutionID),
		})
		if err != nil {
			return fmt.Errorf("failed to get pipeline execution details: %v", err)
		}

		artifact := res.PipelineExecution.ArtifactRevisions[0]
		commitID := aws.StringValue(artifact.RevisionId)
		revisionURL := aws.StringValue(artifact.RevisionUrl)

		var repoID string
		if strings.Contains(revisionURL, "FullRepositoryId=") {
			repoID = strings.Split(strings.Split(revisionURL, "FullRepositoryId=")[1], "&")[0]
		} else {
			segments := strings.Split(revisionURL, "/")
			repoID = segments[3] + "/" + segments[4]
		}

		var state string
		switch strings.ToUpper(message.Detail.State) {
		case "SUCCEEDED":
			state = "success"
		case "RESUMED", "STARTED", "STOPPING", "STOPPED", "SUPERSEDED":
			state = "pending"
		default:
			state = "error"
		}

		region := os.Getenv("AWS_REGION")
		linkURL := fmt.Sprintf("https://%s.console.aws.amazon.com/codesuite/codepipeline/pipelines/%s/executions/%s?region=%s", region, message.Detail.Pipeline, message.Detail.ExecutionID, region)

		if err := updateGitHubPipelineState(repoID, commitID, state, message.Detail.Pipeline, linkURL, ghTokenArn); err != nil {
			return fmt.Errorf("failed to update GitHub pipeline state: %v", err)
		}
	}

	return nil
}

func updateGitHubPipelineState(repoID, commitID, state, context, targetURL, ghTokenArn string) error {
	githubToken, err := getParam("/wardove/GITHUB_TOKEN")
	if err != nil {
		return fmt.Errorf("failed to get secret value: %v", err)
	}
	token := aws.StringValue(&githubToken)
	client := &http.Client{}
	url := fmt.Sprintf("https://api.github.com/repos/%s/statuses/%s", repoID, commitID)
	payload := map[string]interface{}{
		"context":    context,
		"state":      state,
		"target_url": targetURL,
	}

	bytesPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bytesPayload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("token %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub API returned unexpected status code: %d", resp.StatusCode)
	}

	return nil

}

func main() {
	lambda.Start(handler)
}
