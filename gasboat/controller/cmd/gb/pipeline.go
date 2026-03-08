package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gasboat/controller/internal/bridge"

	"github.com/spf13/cobra"
)

var pipelineCmd = &cobra.Command{
	Use:     "pipeline",
	Short:   "GitLab CI pipeline commands",
	GroupID: "orchestration",
}

var pipelineArtifactsCmd = &cobra.Command{
	Use:   "artifacts <bead-id-or-mr-url>",
	Short: "List or download pipeline artifacts from the MR's latest pipeline",
	Args:  cobra.ExactArgs(1),
	RunE:  runPipelineArtifacts,
}

var pipelineLogCmd = &cobra.Command{
	Use:   "log <bead-id-or-mr-url> [job-name]",
	Short: "Fetch job log output from the MR's pipeline",
	Long: `Fetch the log output for a CI job in the MR's latest pipeline.

If job-name is provided, show that specific job's log.
If omitted and the pipeline has a failed job, show the first failed job's log.
If no jobs have failed, list available jobs.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runPipelineLog,
}

func init() {
	pipelineArtifactsCmd.Flags().Bool("download", false, "Download artifacts instead of listing")
	pipelineArtifactsCmd.Flags().StringP("output-dir", "o", ".", "Directory to save downloaded artifacts")

	pipelineCmd.AddCommand(pipelineArtifactsCmd)
	pipelineCmd.AddCommand(pipelineLogCmd)
}

// pipelineURLPattern matches GitLab pipeline URLs:
//
//	https://gitlab.com/org/repo/-/pipelines/12345
var pipelineURLPattern = regexp.MustCompile(`^https?://[^/]+/(.+?)/-/pipelines/(\d+)`)

// resolvePipelineInfo resolves a bead ID or MR URL to (projectPath, pipelineID).
func resolvePipelineInfo(cmd *cobra.Command, arg string) (projectPath string, pipelineID int, err error) {
	client, cErr := newMRGitLabClient()
	if cErr != nil {
		return "", 0, cErr
	}

	// If it's a pipeline URL, parse directly.
	if m := pipelineURLPattern.FindStringSubmatch(arg); m != nil {
		pid, _ := strconv.Atoi(m[2])
		return m[1], pid, nil
	}

	// Resolve to MR URL (bead ID or MR URL).
	mrURL, rErr := resolveMRURL(cmd, arg)
	if rErr != nil {
		return "", 0, rErr
	}

	ref := bridge.ParseMRURL(mrURL)
	if ref == nil {
		return "", 0, fmt.Errorf("could not parse MR URL: %s", mrURL)
	}

	// Query GitLab for the MR to get pipeline info.
	mr, mErr := client.GetMergeRequestByPath(cmd.Context(), ref.ProjectPath, ref.IID)
	if mErr != nil {
		return "", 0, fmt.Errorf("fetching MR !%d: %w", ref.IID, mErr)
	}

	if mr.HeadPipeline == nil || mr.HeadPipeline.ID == 0 {
		return "", 0, fmt.Errorf("MR !%d has no pipeline", ref.IID)
	}

	return ref.ProjectPath, mr.HeadPipeline.ID, nil
}

func runPipelineArtifacts(cmd *cobra.Command, args []string) error {
	projectPath, pipelineID, err := resolvePipelineInfo(cmd, args[0])
	if err != nil {
		return err
	}

	client, err := newMRGitLabClient()
	if err != nil {
		return err
	}

	jobs, err := client.ListPipelineJobsByPath(cmd.Context(), projectPath, pipelineID)
	if err != nil {
		return fmt.Errorf("listing jobs: %w", err)
	}

	// Filter jobs that have artifacts.
	type artifactEntry struct {
		JobID    int    `json:"job_id"`
		JobName  string `json:"job_name"`
		FileType string `json:"file_type"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	}

	var entries []artifactEntry
	for _, job := range jobs {
		for _, a := range job.Artifacts {
			if a.Size > 0 {
				entries = append(entries, artifactEntry{
					JobID:    job.ID,
					JobName:  job.Name,
					FileType: a.FileType,
					Filename: a.Filename,
					Size:     a.Size,
				})
			}
		}
	}

	download, _ := cmd.Flags().GetBool("download")

	if !download {
		// List mode.
		if len(entries) == 0 {
			if jsonOutput {
				printJSON([]any{})
			} else {
				cmd.Printf("No artifacts for pipeline #%d\n", pipelineID)
			}
			return nil
		}

		if jsonOutput {
			printJSON(entries)
			return nil
		}

		cmd.Printf("%-30s %-15s %-12s %10s\n", "JOB", "FILE TYPE", "FILENAME", "SIZE")
		cmd.Printf("%-30s %-15s %-12s %10s\n",
			strings.Repeat("-", 30), strings.Repeat("-", 15), strings.Repeat("-", 12), strings.Repeat("-", 10))
		for _, e := range entries {
			cmd.Printf("%-30s %-15s %-12s %10s\n",
				truncateStr(e.JobName, 30), e.FileType, truncateStr(e.Filename, 12), formatSize(e.Size))
		}
		return nil
	}

	// Download mode.
	if len(entries) == 0 {
		cmd.Printf("No artifacts to download for pipeline #%d\n", pipelineID)
		return nil
	}

	outputDir, _ := cmd.Flags().GetString("output-dir")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Download artifacts per job (one archive per job).
	downloaded := make(map[int]bool)
	for _, e := range entries {
		if downloaded[e.JobID] {
			continue
		}
		downloaded[e.JobID] = true

		filename := fmt.Sprintf("%s-artifacts.zip", e.JobName)
		destPath := filepath.Join(outputDir, filename)

		f, fErr := os.Create(destPath)
		if fErr != nil {
			return fmt.Errorf("creating %s: %w", filename, fErr)
		}

		if dErr := client.DownloadJobArtifacts(cmd.Context(), projectPath, e.JobID, f); dErr != nil {
			f.Close()
			os.Remove(destPath)
			return fmt.Errorf("downloading artifacts for job %s: %w", e.JobName, dErr)
		}
		f.Close()

		cmd.Printf("Downloaded %s\n", destPath)
	}

	return nil
}

func runPipelineLog(cmd *cobra.Command, args []string) error {
	projectPath, pipelineID, err := resolvePipelineInfo(cmd, args[0])
	if err != nil {
		return err
	}

	client, err := newMRGitLabClient()
	if err != nil {
		return err
	}

	jobs, err := client.ListPipelineJobsByPath(cmd.Context(), projectPath, pipelineID)
	if err != nil {
		return fmt.Errorf("listing jobs: %w", err)
	}

	if len(jobs) == 0 {
		cmd.Printf("No jobs found for pipeline #%d\n", pipelineID)
		return nil
	}

	var targetJob *bridge.GitLabJob

	if len(args) > 1 {
		// Find the named job.
		jobName := args[1]
		for i := range jobs {
			if jobs[i].Name == jobName {
				targetJob = &jobs[i]
				break
			}
		}
		if targetJob == nil {
			cmd.Printf("Job %q not found in pipeline #%d. Available jobs:\n", jobName, pipelineID)
			for _, j := range jobs {
				cmd.Printf("  %s (%s) — %s\n", j.Name, j.Stage, j.Status)
			}
			return nil
		}
	} else {
		// Auto-select: first failed job, or list all.
		for i := range jobs {
			if jobs[i].Status == "failed" {
				targetJob = &jobs[i]
				break
			}
		}
		if targetJob == nil {
			if jsonOutput {
				type jobSummary struct {
					ID     int    `json:"id"`
					Name   string `json:"name"`
					Stage  string `json:"stage"`
					Status string `json:"status"`
				}
				var summaries []jobSummary
				for _, j := range jobs {
					summaries = append(summaries, jobSummary{j.ID, j.Name, j.Stage, j.Status})
				}
				printJSON(summaries)
				return nil
			}
			cmd.Printf("No failed jobs in pipeline #%d. Available jobs:\n", pipelineID)
			for _, j := range jobs {
				cmd.Printf("  %-30s %-15s %s\n", j.Name, j.Stage, j.Status)
			}
			return nil
		}
	}

	cmd.Printf("Fetching log for job %q (%s, %s)...\n", targetJob.Name, targetJob.Stage, targetJob.Status)

	log, err := client.GetJobLog(cmd.Context(), projectPath, targetJob.ID)
	if err != nil {
		return fmt.Errorf("fetching job log: %w", err)
	}

	fmt.Println(log)
	return nil
}
