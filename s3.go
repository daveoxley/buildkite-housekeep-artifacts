package main

import (
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/s3"
    "log"
)

func DeleteS3Folder(key string, housekeepConfig HousekeepConfig) {

    svc := s3.New(session.New())

    if housekeepConfig.isVerbose() {
        log.Printf("Getting %s from %s", key, housekeepConfig.bucket)
    }
    params := &s3.ListObjectsInput{
        Bucket:            aws.String(housekeepConfig.bucket),
        Prefix:            aws.String(key + "/"),
    }
    err := svc.ListObjectsPages(params, func(page *s3.ListObjectsOutput, lastPage bool) bool {
        for _, obj := range page.Contents {
            if housekeepConfig.dryRun {
                if housekeepConfig.isInfo() {
                    log.Printf("      Would delete %s", *obj.Key)
                }
            } else {
                if housekeepConfig.isVerbose() {
                    log.Printf("      Deleting %s", *obj.Key)
                }
                req := &s3.DeleteObjectInput{
		    Bucket: aws.String(housekeepConfig.bucket),
		    Key:    obj.Key,
		}
		_, err := svc.DeleteObject(req)
                if err != nil {
                    log.Fatalf("Error %s", err)
                    return false
                }
            }
        }

        return true
    })
    if err != nil {
        log.Fatalf("Error %s", err)
        return
    }
}