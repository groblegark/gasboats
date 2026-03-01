package main

import "github.com/spf13/cobra"

var bundleCmd = &cobra.Command{
	Use:     "bundle",
	Short:   "Manage instantiated template bundles",
	Long:    "Bundles are work item sets created by applying a template.\nEach bundle is an epic-like bead with child beads for each step.",
	GroupID: "beads",
}

func init() {
	bundleCmd.AddCommand(bundleListCmd)
	bundleCmd.AddCommand(bundleShowCmd)
}
