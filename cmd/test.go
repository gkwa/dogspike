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
	"sync"

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
		test()
	},
}

var (
	concurrency int
	lock        sync.Mutex
)

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

// Modify the SuccessBucket struct to include metrics
type SuccessBucket struct {
	Name      string
	ItemCount int64
	TotalSize int64
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
		if obj.Size != nil {
			totalSize += *obj.Size
		}
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
	// Open the file
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

func readSuccessBucketsFromFile() ([]SuccessBucket, error) {
	// Open the file
	file, err := os.OpenFile("success_buckets.json", os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Decode the JSON data from file
	var successBuckets []SuccessBucket
	err = json.NewDecoder(file).Decode(&successBuckets)
	if err != nil {
		return nil, err
	}

	return successBuckets, nil
}

func writeSuccessBucketsToFile(successBuckets []SuccessBucket) error {
	// Create or truncate the file
	file, err := os.OpenFile("success_buckets.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.MarshalIndent(successBuckets, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func containsBucket(bucketList interface{}, bucketName string) bool {
	switch bucketList := bucketList.(type) {
	case []FailedBucket:
		for _, bucket := range bucketList {
			if bucket.Name == bucketName {
				return true
			}
		}
	case []SuccessBucket:
		for _, bucket := range bucketList {
			if bucket.Name == bucketName {
				return true
			}
		}
	}
	return false
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatInt(bytes, 10)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.Flags().IntVarP(&concurrency, "concurrency", "c", 7, "Number of concurrent bucket queries")
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

	// Read the list of failed and successful buckets from file
	failedBuckets, err := readFailedBucketsFromFile()
	if err != nil {
		fmt.Println("Failed to read failed buckets from file:", err)
	}

	successBuckets, err := readSuccessBucketsFromFile()
	if err != nil {
		fmt.Println("Failed to read success buckets from file:", err)
	}

	// Create a slice to hold the bucket information
	bucketList := make([]BucketInfo, 0)

	// Create a channel to receive bucket queries
	bucketChan := make(chan string)

	// Create a wait group to synchronize goroutines
	var wg sync.WaitGroup

	// Start worker goroutines to process bucket queries
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for bucketName := range bucketChan {
				itemCount, totalSize, err := getBucketInfo(s3Client, bucketName)
				if err != nil {
					fmt.Printf("Failed to retrieve objects for bucket '%s': %v\n", bucketName, err)
					// Add the bucket to the failed buckets list
					failedBuckets = append(failedBuckets, FailedBucket{Name: bucketName, Failure: err.Error()})
					continue
				}

				// Create a BucketInfo struct and add it to the bucketList
				bucketInfo := BucketInfo{
					Name:      bucketName,
					ItemCount: itemCount,
					TotalSize: totalSize,
				}

				// Lock the bucketList to safely append the bucketInfo
				// to the list concurrently
				lock.Lock()
				bucketList = append(bucketList, bucketInfo)
				lock.Unlock()

				// Add the bucket to the success buckets list with metrics
				successBucket := SuccessBucket{
					Name:      bucketName,
					ItemCount: itemCount,
					TotalSize: totalSize,
				}

				// Lock the successBuckets to safely append the successBucket
				// to the list concurrently
				lock.Lock()
				successBuckets = append(successBuckets, successBucket)
				lock.Unlock()
			}
		}()
	}

	// Push bucket names to the channel for processing
	for _, bucket := range resp.Buckets {
		bucketName := *bucket.Name

		// Skip the bucket if it is in the failed buckets list
		if containsBucket(failedBuckets, *bucket.Name) || containsBucket(successBuckets, *bucket.Name) {
			continue
		}

		// Skip the bucket if it is in the success buckets list
		if containsBucket(successBuckets, bucketName) {
			continue
		}

		bucketChan <- bucketName
	}

	// Close the bucket channel to signal completion
	close(bucketChan)

	// Wait for all worker goroutines to finish
	wg.Wait()

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

	// Write the updated failed and success buckets lists to file
	err = writeFailedBucketsToFile(failedBuckets)
	if err != nil {
		fmt.Println("Failed to write failed buckets to file:", err)
	}

	err = writeSuccessBucketsToFile(successBuckets)
	if err != nil {
		fmt.Println("Failed to write success buckets to file:", err)
	}
}
