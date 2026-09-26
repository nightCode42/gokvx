package main

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nightCode42/gokvx/internal/config"
)

// newRootCommand builds the gokvx command tree. environ is the process
// environment, from which configuration variables are read.
func newRootCommand(environ []string) *cobra.Command {
	root := &cobra.Command{
		Use:   "gokvx",
		Short: "A strongly consistent, sharded key-value store",
		Long: "gokvx runs a node of a strongly consistent, sharded key-value store.\n\n" +
			"Configuration comes from built-in defaults, an optional YAML file (--config),\n" +
			"GOKVX_* environment variables, and flags, each overriding the one before.\n" +
			"See docs/configuration.md for every setting.",
		Args:          cobra.NoArgs,
		SilenceErrors: true, // run reports errors, with field violations listed
		SilenceUsage:  true, // a failed command is not a usage mistake
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(newVersionCommand(), newConfigCommand(environ))
	return root
}

// newVersionCommand builds `gokvx version`, which reports the build
// (KV-CFG-012).
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit, build date, and Go version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return write(cmd, fmt.Sprintf("gokvx %s\ncommit: %s\nbuilt:  %s\ngo:     %s\n",
				Version, Commit, BuildDate, runtime.Version()))
		},
	}
}

// write prints text to the command's output.
func write(cmd *cobra.Command, text string) error {
	if _, err := io.WriteString(cmd.OutOrStdout(), text); err != nil {
		return fmt.Errorf("main.write: %w", err)
	}
	return nil
}

// newConfigCommand builds `gokvx config` and its subcommands.
func newConfigCommand(environ []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Work with node configuration",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newConfigValidateCommand(environ))
	return cmd
}

// newConfigValidateCommand builds `gokvx config validate`, which checks a
// configuration without starting a node (KV-CFG-003).
func newConfigValidateCommand(environ []string) *cobra.Command {
	var (
		file           string
		printEffective bool
	)
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check a configuration without starting a node",
		Long: "Validate applies the configuration file, GOKVX_* environment variables, and\n" +
			"flags exactly as a starting node would, then reports every violation by key.\n" +
			"It exits non-zero if the configuration is invalid.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := validateConfig(config.Options{File: file, Environ: environ, Flags: cmd.Flags()}, printEffective)
			if err != nil {
				return err
			}
			return write(cmd, report)
		},
	}
	cmd.Flags().StringVar(&file, "config", "", "path of a YAML configuration file")
	cmd.Flags().BoolVar(&printEffective, "print", false, "also print the effective configuration, with credentials redacted")
	if err := config.RegisterFlags(cmd.Flags()); err != nil {
		// Unreachable unless a configuration key has an unsupported type,
		// which the config package's tests rule out; fail the command rather
		// than the process.
		cmd.RunE = func(*cobra.Command, []string) error {
			return fmt.Errorf("main.newConfigValidateCommand: %w", err)
		}
	}
	return cmd
}

// validateConfig loads and validates a configuration and returns the report
// to print: that it is valid, each unsafe setting, and optionally the
// effective configuration with credentials redacted.
func validateConfig(opts config.Options, printEffective bool) (string, error) {
	cfg, err := config.Load(opts)
	if err != nil {
		return "", fmt.Errorf("main.validateConfig: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("main.validateConfig: %w", err)
	}

	var report strings.Builder
	report.WriteString("configuration is valid\n")
	for _, w := range cfg.Warnings() {
		report.WriteString("warning: " + w.String() + "\n")
	}
	if printEffective {
		redacted := cfg.Redacted()
		text, err := redacted.YAML()
		if err != nil {
			return "", fmt.Errorf("main.validateConfig: %w", err)
		}
		report.WriteString("\n")
		report.Write(text)
	}
	return report.String(), nil
}
