// main.go
package main

import (
	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{Use: "cluster-reflector-cli"}

	rootCmd.Execute()
}
