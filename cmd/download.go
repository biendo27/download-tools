package cmd

import (
	"fmt"
	"gdl/pkg/downloader"
	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:   "download [url]",
	Short: "Download a file from URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		url := args[0]
		concurrency, _ := cmd.Flags().GetInt("concurrency")
		output, _ := cmd.Flags().GetString("output")
		dir, _ := cmd.Flags().GetString("dir")

		d := downloader.NewDownloader()
		err := d.Download(downloader.DownloadConfig{
			Url:         url,
			Concurrency: concurrency,
			OutputName:  output,
			OutputDir:   dir,
		})
		if err != nil {
			fmt.Println("Error:", err)
		}
	},
}

func init() {
	downloadCmd.Flags().IntP("concurrency", "c", 16, "Number of concurrent connections")
	downloadCmd.Flags().StringP("output", "o", "", "Output filename")
	downloadCmd.Flags().StringP("dir", "d", "", "Output directory")
	rootCmd.AddCommand(downloadCmd)
}
