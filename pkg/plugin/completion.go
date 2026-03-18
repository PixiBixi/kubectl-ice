package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var completionShort = "Generate shell completion scripts"

var completionDescription = ` Outputs a shell completion script for the specified shell.

Source the script in your shell profile to enable tab completion for all
kubectl-ice subcommands, flags, and arguments.

For kubectl plugin usage (kubectl ice <TAB>), create a symlink or wrapper
named kubectl_complete-ice pointing to kubectl-ice in your PATH.`

var completionExample = `  # Zsh: add to ~/.zshrc
  source <(kubectl-ice completion zsh)

  # Zsh: install persistently (recommended)
  kubectl-ice completion zsh --install

  # Bash: add to ~/.bashrc
  source <(kubectl-ice completion bash)

  # Fish
  kubectl-ice completion fish | source

  # kubectl plugin integration (kubectl ice <TAB>)
  ln -s $(which kubectl-ice) $(dirname $(which kubectl-ice))/kubectl_complete-ice`

func Completion(rootCmd *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish]",
		Short:     completionShort,
		Long:      fmt.Sprintf("%s\n\n%s", completionShort, completionDescription),
		Example:   completionExample,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			install, _ := cmd.Flags().GetBool("install")
			switch args[0] {
			case "zsh":
				return runZshCompletion(rootCmd, install)
			case "bash":
				return runBashCompletion(rootCmd, install)
			case "fish":
				return runFishCompletion(rootCmd, install)
			default:
				return fmt.Errorf("unsupported shell %q: choose bash, zsh, or fish", args[0])
			}
		},
	}

	cmd.Flags().BoolP("install", "i", false, "install the completion script to the standard location for the shell")

	return cmd
}

func runZshCompletion(rootCmd *cobra.Command, install bool) error {
	if !install {
		return rootCmd.GenZshCompletion(os.Stdout)
	}

	// Prefer the first writable dir in $fpath, fall back to ~/.zsh/completions.
	dir := zshCompletionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating completion dir %s: %w", dir, err)
	}

	dest := filepath.Join(dir, "_kubectl-ice")
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("writing completion file: %w", err)
	}
	defer f.Close()

	if err := rootCmd.GenZshCompletion(f); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Installed zsh completion to %s\n", dest)
	fmt.Fprintln(os.Stderr, "Restart your shell or run: autoload -Uz compinit && compinit")
	return nil
}

func runBashCompletion(rootCmd *cobra.Command, install bool) error {
	if !install {
		return rootCmd.GenBashCompletion(os.Stdout)
	}

	dir := "/etc/bash_completion.d"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".bash_completion.d")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating completion dir %s: %w", dir, err)
		}
	}

	dest := filepath.Join(dir, "kubectl-ice")
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("writing completion file: %w", err)
	}
	defer f.Close()

	if err := rootCmd.GenBashCompletion(f); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Installed bash completion to %s\n", dest)
	fmt.Fprintln(os.Stderr, "Restart your shell or run: source ~/.bash_completion.d/kubectl-ice")
	return nil
}

func runFishCompletion(rootCmd *cobra.Command, install bool) error {
	if !install {
		return rootCmd.GenFishCompletion(os.Stdout, true)
	}

	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "fish", "completions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating completion dir %s: %w", dir, err)
	}

	dest := filepath.Join(dir, "kubectl-ice.fish")
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("writing completion file: %w", err)
	}
	defer f.Close()

	if err := rootCmd.GenFishCompletion(f, true); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Installed fish completion to %s\n", dest)
	return nil
}

// zshCompletionDir returns ~/.zsh/completions, creating it if needed.
// A common convention for user-local zsh completions — add to fpath in ~/.zshrc:
// fpath=(~/.zsh/completions $fpath)
func zshCompletionDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zsh", "completions")
}
