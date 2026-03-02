package main

import "github.com/spf13/cobra"

var molCmd = &cobra.Command{
	Use:     "mol",
	Short:   "Manage instantiated formula molecules",
	Long:    "Molecules are work item sets created by pouring a formula.\nEach molecule is an epic-like bead with child beads for each step.",
	GroupID: "beads",
}

// bundleCmd is a hidden alias for backward compatibility.
var bundleCmd = &cobra.Command{
	Use:     "bundle",
	Short:   "Manage instantiated template bundles (deprecated: use 'mol')",
	Long:    "Deprecated — use 'kd mol' instead.\n\nMolecules are work item sets created by pouring a formula.\nEach molecule is an epic-like bead with child beads for each step.",
	GroupID: "beads",
	Hidden:  true,
}

func init() {
	// Register on hidden alias first, then primary — Cobra uses last parent
	// for Usage line, so primary command shows the correct path.
	bundleCmd.AddCommand(newPourCmd())
	bundleCmd.AddCommand(newWispCmd())

	molCmd.AddCommand(molListCmd)
	molCmd.AddCommand(molShowCmd)
	molCmd.AddCommand(newPourCmd())
	molCmd.AddCommand(newWispCmd())
}
