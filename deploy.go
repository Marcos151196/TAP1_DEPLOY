package main

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/s3"

	aws "github.com/aws/aws-sdk-go/aws"
	session "github.com/aws/aws-sdk-go/aws/session"
	ec2 "github.com/aws/aws-sdk-go/service/ec2"
	sqs "github.com/aws/aws-sdk-go/service/sqs"
	viper "github.com/spf13/viper"
)

var cfgFile = "config/config.toml"

var sess, _ = session.NewSession(&aws.Config{
	Region: aws.String("eu-west-2")},
)

var ec2svc = ec2.New(sess)
var sqssvc *sqs.SQS = sqs.New(sess)
var s3svc = s3.New(sess)

func main() {
	// CONFIG FILE
	viper.SetConfigFile(cfgFile)
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("[INIT] Unable to read config from file %s: %v\n", cfgFile, err)
		return
	} else {
		fmt.Printf("[INIT] Read configuration from file %s\n", cfgFile)
	}

	// MQTT INSTANCE
	res, err := ec2svc.RunInstances(&ec2.RunInstancesInput{
		ImageId:        aws.String(viper.GetString("mqtt.ami")),
		InstanceType:   aws.String(viper.GetString("mqtt.instancetype")),
		MinCount:       aws.Int64(1),
		MaxCount:       aws.Int64(1),
		KeyName:        aws.String(viper.GetString("general.keypairname")),
		SecurityGroups: aws.StringSlice([]string{viper.GetString("general.secgroup")}),
	})
	if err != nil {
		fmt.Println("Could not create MQTT Broker EC2 instance", err)
		return
	}
	fmt.Println("Created MQTT Broker EC2 instance")

	// Wait until instance is ready
	MQTTinstanceID := *res.Instances[0].InstanceId
	err = ec2svc.WaitUntilInstanceStatusOk(&ec2.DescribeInstanceStatusInput{
		InstanceIds: aws.StringSlice([]string{MQTTinstanceID}),
	})
	if err != nil {
		fmt.Printf("MQTT instance timeout.")
		return
	}
	fmt.Printf("MQTT instance running!")

	// Associate MQTT instance elastic IP
	MQTTIP := viper.GetString("mqtt.IP")
	_, err = ec2svc.AssociateAddress(&ec2.AssociateAddressInput{
		PublicIp:   aws.String(MQTTIP),
		InstanceId: aws.String(MQTTinstanceID),
	})
	if err != nil {
		fmt.Printf("Unable to associate IP address with %s, %v", MQTTinstanceID, err)
	}
	fmt.Printf("Successfully allocated %s with instance %s.\n", MQTTIP, MQTTinstanceID)

	// ECHOSEARCH INSTANCE
	for i := 0; i < viper.GetInt("echosearch.numberofinstances"); i++ {
		runResult, err := ec2svc.RunInstances(&ec2.RunInstancesInput{
			ImageId:        aws.String(viper.GetString("echosearch.ami")),
			InstanceType:   aws.String(viper.GetString("echosearch.instancetype")),
			MinCount:       aws.Int64(1),
			MaxCount:       aws.Int64(1),
			KeyName:        aws.String(viper.GetString("general.keypairname")),
			SecurityGroups: aws.StringSlice([]string{viper.GetString("general.secgroup")}),
		})
		if err != nil {
			fmt.Println("Could not create ECHOSEARCH EC2 instance", err)
			return
		}
		fmt.Println("Created ECHOSEARCH EC2 instance")
		// Add tags to the created instance
		_, errtag := ec2svc.CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{runResult.Instances[0].InstanceId},
			Tags: []*ec2.Tag{
				{
					Key:   aws.String("Type"),
					Value: aws.String("echosearch"),
				},
			},
		})
		if errtag != nil {
			fmt.Println("Could not create tags for instance", runResult.Instances[0].InstanceId, errtag)
			return
		}
		fmt.Println("Successfully tagged instance")
	}

	// SQS INBOX QUEUE
	result, err := sqssvc.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String(viper.GetString("sqs.inboxname")),
		Attributes: map[string]*string{
			"VisibilityTimeout":             aws.String(viper.GetString("sqs.inboxvisibilitytimeout")),
			"DelaySeconds":                  aws.String("0"),
			"ReceiveMessageWaitTimeSeconds": aws.String("0"),
		},
	})
	if err != nil {
		fmt.Println("Could not create SQS Inbox queue.", err)
		return
	}
	fmt.Println("SQS Inbox queue succesfully created.", *result.QueueUrl)

	// SQS OUTBOX QUEUE
	result, err = sqssvc.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String(viper.GetString("sqs.outboxname")),
		Attributes: map[string]*string{
			"VisibilityTimeout":             aws.String(viper.GetString("sqs.outboxvisibilitytimeout")),
			"DelaySeconds":                  aws.String("0"),
			"ReceiveMessageWaitTimeSeconds": aws.String("0"),
		},
	})
	if err != nil {
		fmt.Println("Could not create SQS Outbox queue.", err)
		return
	}
	fmt.Println("SQS Outbox queue succesfully created.", *result.QueueUrl)

	// WEB CLIENT INSTANCE
	res, err = ec2svc.RunInstances(&ec2.RunInstancesInput{
		ImageId:        aws.String(viper.GetString("webclient.ami")),
		InstanceType:   aws.String(viper.GetString("webclient.instancetype")),
		MinCount:       aws.Int64(1),
		MaxCount:       aws.Int64(1),
		KeyName:        aws.String(viper.GetString("general.keypairname")),
		SecurityGroups: aws.StringSlice([]string{viper.GetString("general.secgroup")}),
	})
	if err != nil {
		fmt.Println("Could not create WEB SERVER EC2 instance", err)
		return
	}
	fmt.Println("Created WEB SERVER EC2 instance")

	// Wait until instance is ready
	WEBCLIENTinstanceID := *res.Instances[0].InstanceId
	err = ec2svc.WaitUntilInstanceStatusOk(&ec2.DescribeInstanceStatusInput{
		InstanceIds: aws.StringSlice([]string{WEBCLIENTinstanceID}),
	})
	if err != nil {
		fmt.Printf("WEB SERVER instance timeout.")
		return
	}
	fmt.Printf("MQTT instance running!")

	// Assign the instance an elastic IP
	WebClientIP := viper.GetString("webclient.IP")
	_, err = ec2svc.AssociateAddress(&ec2.AssociateAddressInput{
		PublicIp:   aws.String(WebClientIP),
		InstanceId: aws.String(WEBCLIENTinstanceID),
	})
	if err != nil {
		fmt.Printf("Unable to associate IP address with %s, %v", WEBCLIENTinstanceID, err)
	}
	fmt.Printf("Successfully allocated %s with instance %s.\n", WebClientIP, WEBCLIENTinstanceID)

	// CREATE S3 BUCKET AND conversations FOLDER
	_, err = s3svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(viper.GetString("s3.bucketname")),
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String("eu-west-2"),
		},
	})
	if err != nil {
		fmt.Println("Could not create s3 bucket: ", err)
		return
	}

	_, err = s3svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(viper.GetString("s3.bucketname")),
		Key:    aws.String("/conversations/init.txt"),
	})
	if err != nil {
		fmt.Println("Could not create conversations folder: ", err)
		return
	}
	fmt.Println("S3 bucket tap1 and conversations folder created!")

}
