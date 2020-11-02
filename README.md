# awsCodePipelineDeployer
Eases deploying artifacts to S3 buckets belonging to other AWS accounts

The official procedure to do this is this: https://aws.amazon.com/premiumsupport/knowledge-center/codepipeline-artifacts-s3/, but this is very complicated to set up, because you need to set up a lot of roles, keys, etc.

Instead, you can create a lambda function using this code to upload the artifacts to the "foreign" S3 bucket.

## Requirements
To compile the function, you need go 1.15.
The pipeline and the destination S3 bucket must be in the same region.

## Definitions
The "dev" account is the AWS account where you have your AWS CodePipeline pipeline
The "foreign" account is the AWS account where you want your artifacts to be uploaded to.

## Compile and package for AWS Lambda (linux):
```
go build
zip awsCodePipelineDeployer.zip awsCodePipelineDeployer
```

## Compile and package for AWS Lambda (windows):
```
go get -u github.com/aws/aws-lambda-go/cmd/build-lambda-zip
set GOOS=linux
go build
%USERPROFILE%\go\bin\build-lambda-zip.exe -output awsCodePipelineDeployer.zip awsCodePipelineDeployer
```

## Create a user in the "foreign" account
You need to create a user in the foreign AWS account with "Programmatic access". It should have write permissions to the S3 bucket you want to put the artifacts in. Store the access key id and the secret access key somewhere safe. You are going to need it when setting up the lambda function.

Here is an example of a policy document that works:
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:AbortMultipartUpload",
                "s3:DeleteObjectVersion",
                "s3:DeleteObject"
            ],
            "Resource": "arn:aws:s3:::<bucket>/<optional prefix>/*"
        }
    ]
}
```

## Create the lambda function in the "dev" account
Create a new lambda function in the AWS console - give it a meaningful name. Select go1.x as runtime.

In permissions, select "Create a new role with basic Lambda permissions". Then you press "Create function" and upload the awsCodePipelineDeployer.zip you generated earlier as function code. In basic settings, you need to change Handler to awsCodePipelineDeployer. You should also consider extending the timeout to something reasonable depending on how large your artifacts are.

Now you need to configure the function. That is done in the Environment variables section. You need to set up the following variables:
- REGION = &lt;your region&gt;
- BUCKET = &lt;destination bucket name&gt;
- PREFIX = &lt;optional prefix&gt; - if you want the artifacts to be uploaded in a directory, enter this here. Remember to prefix with a slash - '/'
- ACCESSKEYID = &lt;the access key ID from the user you generated&gt;
- SECRETACCESSKEY = &lt;the secret access key from the user you generated&gt;

Finally, you have to create a new policy that you have to attach to the role the lambda function is using. You can see the name of the role in the Permissions tab of the lambda function. The new policy grants the lambda function the ability to report back to AWS CodePipeline that the deployment has succeeded/failed:

Here is a policy document you can use. Remember to attach the policy to the role used by the lambda function.
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "codepipeline:PutJobFailureResult",
                "codepipeline:PutJobSuccessResult"
            ],
            "Resource": "*"
        }
    ]
}
```

## Modifying the pipeline
Finally, you need to tell your pipeline to use the lambda function. Edit your pipeline. If you already have a Deploy stage, you can add an action in that one - or add a new stage to the pipeline. When adding the action, you need to select AWS Lambda as Action provider. Select the input artifacts you want to deploy and select the lambda function in the "Function name" dropdown. The function does not output any artifacts, so leave the "Variable namespace" and "Output artifacts" empty. Save your changes and you should have a new step that calls the lambda function.

## What the function actually does
The lambda function downloads the input artifact, unizps it and uploads the files from the .zip-file to the destination bucket specified. By providing the credentials from the "foreign" account in the environment variables, it can upload files to the foreign S3 bucket without any problems.
