package main

import "github.com/spf13/cobra"

var formulaCmd = &cobra.Command{
	Use:     "formula",
	Short:   "Manage reusable work formulas",
	Long:    "Formulas define reusable sets of work items (steps) with variable substitution.\nCreate a formula, then pour it to instantiate a molecule of beads.",
	GroupID: "beads",
}

// templateCmd is a hidden alias for backward compatibility.
var templateCmd = &cobra.Command{
	Use:     "template",
	Short:   "Manage reusable work templates (deprecated: use 'formula')",
	Long:    "Deprecated — use 'kd formula' instead.\n\nFormulas define reusable sets of work items (steps) with variable substitution.\nCreate a formula, then pour it to instantiate a molecule of beads.",
	GroupID: "beads",
	Hidden:  true,
}

func init() {
	// Register on hidden alias first, then primary — Cobra uses last parent
	// for Usage line, so primary command shows the correct path.
	templateCmd.AddCommand(newPourCmd())
	templateCmd.AddCommand(newWispCmd())

	formulaCmd.AddCommand(formulaCreateCmd)
	formulaCmd.AddCommand(formulaListCmd)
	formulaCmd.AddCommand(formulaShowCmd)
	formulaCmd.AddCommand(formulaApplyCmd)
	formulaCmd.AddCommand(newPourCmd())
	formulaCmd.AddCommand(newWispCmd())
}
