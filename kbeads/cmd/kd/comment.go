package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var commentCmd = &cobra.Command{
	Use:     "comment",
	Short:   "Manage bead comments",
	GroupID: "beads",
}

var commentAddCmd = &cobra.Command{
	Use:   "add <bead-id> <text>...",
	Short: "Add a comment to a bead",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]
		text := strings.Join(args[1:], " ")

		c, err := beadsClient.AddComment(context.Background(), beadID, actor, text)
		if err != nil {
			return fmt.Errorf("adding comment: %w", err)
		}

		if jsonOutput {
			data, err := json.MarshalIndent(c, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(data))
		} else {
			fmt.Printf("ID:         %d\n", c.ID)
			fmt.Printf("Bead:       %s\n", c.BeadID)
			fmt.Printf("Author:     %s\n", c.Author)
			fmt.Printf("Text:       %s\n", c.Text)
			if !c.CreatedAt.IsZero() {
				fmt.Printf("Created At: %s\n", c.CreatedAt.Format("2006-01-02 15:04:05"))
			}
		}
		return nil
	},
}

var commentListCmd = &cobra.Command{
	Use:   "list <bead-id>",
	Short: "List comments on a bead",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]

		comments, err := beadsClient.GetComments(context.Background(), beadID)
		if err != nil {
			return fmt.Errorf("listing comments: %w", err)
		}

		if jsonOutput {
			data, err := json.MarshalIndent(comments, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(data))
		} else {
			if len(comments) == 0 {
				fmt.Println("No comments found.")
				return nil
			}
			for i, c := range comments {
				if i > 0 {
					fmt.Println("---")
				}
				createdAt := ""
				if !c.CreatedAt.IsZero() {
					createdAt = c.CreatedAt.Format("2006-01-02 15:04:05")
				}
				fmt.Printf("[%s] %s:\n  %s\n", createdAt, c.Author, c.Text)
			}
		}
		return nil
	},
}

func init() {
	commentCmd.AddCommand(commentAddCmd)
	commentCmd.AddCommand(commentListCmd)
}
