package main

import (
	"archive/zip"
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/s3manager"
	"io/ioutil"
	"log"
	"os"
)

func uploadFile(ctx context.Context, source *zip.File, bucket, prefix string, uploader *s3manager.Uploader) error {
	fileContents, err := source.Open()
	if err != nil {
		return err
	}
	defer fileContents.Close()

	_, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: &bucket,
		Key:    aws.String(prefix + source.Name),
		Body:   fileContents,
	})
	return err
}

func copyArtifact(ctx context.Context, artifact events.CodePipelineInputArtifact, destBucket, destPrefix string, downloader *s3manager.Downloader, uploader *s3manager.Uploader) error {
	if artifact.Location.LocationType != "S3" {
		return nil
	}

	tempFile, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	bytesRead, err := downloader.DownloadWithContext(ctx, tempFile, &s3.GetObjectInput{
		Bucket: &artifact.Location.S3Location.BucketName,
		Key:    &artifact.Location.S3Location.ObjectKey,
	})
	if err != nil {
		return err
	}

	zipfile, err := zip.NewReader(tempFile, bytesRead)
	if err != nil {
		return err
	}

	for _, file := range zipfile.File {
		if err = uploadFile(ctx, file, destBucket, destPrefix, uploader); err != nil {
			return err
		}
	}
	return nil
}

func run(ctx context.Context, event events.CodePipelineEvent) error {
	region := os.Getenv("REGION")

	sourceCfg, err := external.LoadDefaultAWSConfig(
		external.WithRegion(region),
		external.WithCredentialsProvider{
			aws.NewStaticCredentialsProvider(
				event.CodePipelineJob.Data.ArtifactCredentials.AccessKeyID,
				event.CodePipelineJob.Data.ArtifactCredentials.SecretAccessKey,
				event.CodePipelineJob.Data.ArtifactCredentials.SessionToken),
		})

	if err != nil {
		return err
	}

	destCfg, err := external.LoadDefaultAWSConfig(
		external.WithRegion(region),
		external.WithCredentialsProvider{
			aws.NewStaticCredentialsProvider(
				os.Getenv("ACCESSKEYID"),
				os.Getenv("SECRETACCESSKEY"),
				""),
		})
	if err != nil {
		return err
	}

	destBucket := os.Getenv("BUCKET")
	destPrefix := os.Getenv("PREFIX")

	downloader := s3manager.NewDownloader(sourceCfg)
	uploader := s3manager.NewUploader(destCfg)

	for _, artifact := range event.CodePipelineJob.Data.InputArtifacts {
		if err = copyArtifact(ctx, artifact, destBucket, destPrefix, downloader, uploader); err != nil {
			return err
		}
	}

	return nil
}

var cpClient *codepipeline.Client

func HandleRequest(ctx context.Context, event events.CodePipelineEvent) (string, error) {
	var err error
	if err = run(ctx, event); err != nil {
		if _, reportErr := cpClient.PutJobFailureResultRequest(&codepipeline.PutJobFailureResultInput{
			JobId: &event.CodePipelineJob.ID,
			FailureDetails: &codepipeline.FailureDetails{
				Type:    codepipeline.FailureTypeJobFailed,
				Message: aws.String(err.Error()),
			},
		}).Send(ctx); reportErr != nil {
			err = fmt.Errorf("Failure reporting back to codepipeline: %w. The original error was %s", reportErr, err.Error())
		}
		return "Completed", err
	}

	_, err = cpClient.PutJobSuccessResultRequest(&codepipeline.PutJobSuccessResultInput{
		JobId: &event.CodePipelineJob.ID,
	}).Send(ctx)
	return "Completed", err
}

func main() {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		log.Fatal(err)
	}
	cpClient = codepipeline.New(cfg)

	lambda.Start(HandleRequest)
}
