/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

// Define a struct to hold failed bucket retrievals
type FailedBucket struct {
	Name    string
	Failure string
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
	resp, err := s3Client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		fmt.Println("Failed to retrieve S3 buckets:", err)
		return
	}

	// Read the list of failed buckets from file
	failedBuckets, err := readFailedBucketsFromFile()
	if err != nil {
		fmt.Println("Failed to read failed buckets from file:", err)
		// return
	}

	// Create a slice to hold the bucket information
	bucketList := make([]BucketInfo, 0)

	// Iterate over each bucket
	for _, bucket := range resp.Buckets {
		// Skip the bucket if it is in the failed buckets list
		if containsBucket(failedBuckets, *bucket.Name) {
			continue
		}

		itemCount, totalSize, err := getBucketInfo(s3Client, *bucket.Name)
		if err != nil {
			fmt.Printf("Failed to retrieve objects for bucket '%s': %v\n", *bucket.Name, err)
			// Add the bucket to the failed buckets list
			failedBuckets = append(failedBuckets, FailedBucket{Name: *bucket.Name, Failure: err.Error()})
			continue
		}

		// Create a BucketInfo struct and add it to the bucketList
		bucketInfo := BucketInfo{
			Name:      *bucket.Name,
			ItemCount: itemCount,
			TotalSize: totalSize,
		}

		bucketList = append(bucketList, bucketInfo)
	}

	// Sort the bucketList by TotalSize in descending order
	sort.Slice(bucketList, func(i, j int) bool {
		return bucketList[i].TotalSize > bucketList[j].TotalSize
	})

	// Print the sorted bucket information
	for _, bucketInfo := range bucketList {
		fmt.Printf("Size: ")
		if bucketInfo.TotalSize >= 1024*1024*1024 {
			fmt.Printf("%.2f GB, ", float64(bucketInfo.TotalSize)/(1024*1024*1024))
		} else if bucketInfo.TotalSize >= 1024*1024 {
			fmt.Printf("%.2f MB, ", float64(bucketInfo.TotalSize)/(1024*1024))
		} else {
			fmt.Printf("%s bytes, ", formatBytes(bucketInfo.TotalSize))
		}

		fmt.Printf("Item Count: %d, ", bucketInfo.ItemCount)
		fmt.Printf("Bucket Name: %s", bucketInfo.Name)
		fmt.Println()
	}

	// Write the updated failed buckets list to file
	err = writeFailedBucketsToFile(failedBuckets)
	if err != nil {
		fmt.Println("Failed to write failed buckets to file:", err)
	}
}

func getBucketInfo(s3Client *s3.Client, bucketName string) (int64, int64, error) {
	// Create the input for ListObjectsV2 operation
	input := &s3.ListObjectsV2Input{
		Bucket: &bucketName,
	}

	// Retrieve the objects in the bucket
	resp, err := s3Client.ListObjectsV2(context.TODO(), input)
	if err != nil {
		return 0, 0, err
	}

	// Calculate the summary information
	itemCount := int64(len(resp.Contents))
	totalSize := int64(0)

	for _, obj := range resp.Contents {
		totalSize += obj.Size
	}

	return itemCount, totalSize, nil
}

func readFailedBucketsFromFile() ([]FailedBucket, error) {
	// Open the file
	file, err := os.OpenFile("failed_buckets.json", os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Decode the JSON data from file
	var failedBuckets []FailedBucket
	err = json.NewDecoder(file).Decode(&failedBuckets)
	if err != nil {
		return nil, err
	}

	return failedBuckets, nil
}

func writeFailedBucketsToFile(failedBuckets []FailedBucket) error {
	// Create or truncate the file
	file, err := os.OpenFile("failed_buckets.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.MarshalIndent(failedBuckets, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func containsBucket(failedBuckets []FailedBucket, bucketName string) bool {
	for _, failedBucket := range failedBuckets {
		if failedBucket.Name == bucketName {
			return true
		}
	}
	return false
}

// Helper function to format the size with thousand separators
func formatBytes(size int64) string {
	str := strconv.FormatInt(size, 10)
	var formattedBytes []string
	for i := len(str) - 1; i >= 0; i-- {
		formattedBytes = append(formattedBytes, string(str[i]))
		if (len(str)-i)%3 == 0 && i != 0 {
			formattedBytes = append(formattedBytes, ",")
		}
	}
	// Reverse the order of characters
	for i, j := 0, len(formattedBytes)-1; i < j; i, j = i+1, j-1 {
		formattedBytes[i], formattedBytes[j] = formattedBytes[j], formattedBytes[i]
	}
	return strings.Join(formattedBytes, "")
}
