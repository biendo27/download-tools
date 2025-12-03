package cmd

import (
	"bufio"
	"fmt"
	"gdl/pkg/downloader"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var batchCmd = &cobra.Command{
	Use:   "batch [file]",
	Short: "Download multiple files from a list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		file, err := os.Open(filePath)
		if err != nil {
			fmt.Println("Error opening file:", err)
			return
		}
		defer file.Close()

		concurrency, _ := cmd.Flags().GetInt("concurrency")
		dir, _ := cmd.Flags().GetString("dir")
		d := downloader.NewDownloader()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			url := strings.TrimSpace(scanner.Text())
			if url == "" || strings.HasPrefix(url, "#") {
				continue
			}
			fmt.Println("Processing:", url)
			err := d.Download(downloader.DownloadConfig{
				Url:         url,
				Concurrency: concurrency,
				OutputDir:   dir,
			})
			if err != nil {
				fmt.Printf("Error downloading %s: %v\n", url, err)
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Println("Error reading file:", err)
		}
	},
}

func init() {
	batchCmd.Flags().IntP("concurrency", "c", 16, "Number of concurrent connections per download")
	batchCmd.Flags().StringP("dir", "d", "", "Output directory")
	rootCmd.AddCommand(batchCmd)
}
