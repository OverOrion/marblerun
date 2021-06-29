package cmd

import (
	"fmt"
	"io/ioutil"

	"github.com/spf13/cobra"
)

func newManifestLog() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "log <IP:PORT>",
		Short: "Get the update log from the Marblerun coordinator",
		Long: `Get the update log from the Marblerun coordinator.
		The log is list of all successful changes to the coordinator,
		including a timestamp and user performing the operation.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hostName := args[0]
			cert, err := verifyCoordinator(hostName, eraConfig, insecureEra)
			if err != nil {
				return err
			}
			fmt.Println("Successfully verified coordinator, now requesting update log")
			response, err := cliDataGet(hostName, "update", "data", cert)
			if err != nil {
				return err
			}
			if len(output) > 0 {
				return ioutil.WriteFile(output, response, 0644)
			}
			fmt.Printf("Update log:\n%s", string(response))
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "Save log to file instead of printing to stdout")
	return cmd
}
