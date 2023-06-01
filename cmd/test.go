/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/spf13/cobra"
)

// testCmd represents the test command
var testCmd = &cobra.Command{
	Use:   "test",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("test called")
		test()
	},
}

func init() {
	rootCmd.AddCommand(testCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// testCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// testCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// Define a struct to hold bucket information
type BucketInfo struct {
	Name      string
	ItemCount int64
	TotalSize int64
}

func test() {
	// Create a new AWS configuration
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-west-2"))
	if err != nil {
		fmt.Println("Failed to load AWS configuration:", err)
		return
	}

	// Create an S3 client
	s3Client := s3.NewFromConfig(cfg)

	// Retrieve the list of S3 buckets in the specified region
	buckets, err := s3Client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		fmt.Println("Failed to retrieve S3 buckets:", err)
		return
	}

	// Get the first two buckets
	var selectedBuckets []types.Bucket

	if len(buckets.Buckets) > 0 {
		selectedBuckets = append(selectedBuckets, buckets.Buckets[0])

		if len(buckets.Buckets) > 1 {
			selectedBuckets = append(selectedBuckets, buckets.Buckets[1])
		}
	}

	// Iterate over each bucket and print its name
	for _, bucket := range selectedBuckets {
		doit2(*bucket.Name)
	}
}

func doit2(bucketName string) {
	// Create a new AWS configuration
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-west-2"))
	if err != nil {
		fmt.Println("Failed to load AWS configuration:", err)
		return
	}

	// Create an S3 client
	s3Client := s3.NewFromConfig(cfg)

	// Create the input for ListObjectsV2 operation
	input := &s3.ListObjectsV2Input{
		Bucket: &bucketName,
	}

	// Retrieve the objects in the bucket
	resp, err := s3Client.ListObjectsV2(context.TODO(), input)
	if err != nil {
		fmt.Println("Failed to retrieve objects:", err)
		return
	}

	// Calculate the summary information
	totalSize := int64(0)
	totalObjects := int64(0)

	for _, obj := range resp.Contents {
		totalSize += obj.Size
		totalObjects++
	}

	fmt.Printf("Summary for bucket '%s':\n", bucketName)
	fmt.Printf("Total Size: %s\n", formatSize(totalSize))
	fmt.Printf("Total Objects: %d\n", totalObjects)
}

// Helper function to format the size in a human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
