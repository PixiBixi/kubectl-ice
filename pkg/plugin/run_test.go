package plugin

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// runTestCommand returns the real cobra command for a subcommand, so the test
// reads the same flags production does rather than a hand rolled flag set.
func runTestCommand(t *testing.T, name string, flags ...string) *cobra.Command {
	t.Helper()

	root := &cobra.Command{Use: "kubectl-ice"}
	InitSubCommands(root)

	cmd, _, err := root.Find([]string{name})
	if err != nil {
		t.Fatalf("subcommand %s not found: %v", name, err)
	}
	if err := cmd.ParseFlags(flags); err != nil {
		t.Fatalf("parsing %v: %v", flags, err)
	}

	return cmd
}

func runTestPods() []*v1.Pod {
	pod := func(name string, images ...string) *v1.Pod {
		p := &v1.Pod{Name: name, Namespace: "team-a"}
		for i, image := range images {
			p.Spec.Containers = append(p.Spec.Containers, v1.Container{
				Name:  []string{"app", "sidecar"}[i],
				Image: image,
			})
			p.Status.ContainerStatuses = append(p.Status.ContainerStatuses, v1.ContainerStatus{
				Name: []string{"app", "sidecar"}[i], Ready: true, Image: image,
			})
		}

		return p
	}

	return []*v1.Pod{
		pod("zulu-pod", "eu.gcr.io/project/app:v2", "docker.io/istio/proxyv2:1.20.0"),
		pod("alpha-pod", "eu.gcr.io/project/app:v1"),
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote. It
// also points os.Stdin at /dev/null, because Build treats any stdin that is not
// a character device as yaml to read and would block on the test harness pipe.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }()

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdout, stdin := os.Stdout, os.Stdin
	os.Stdout, os.Stdin = write, devNull
	t.Cleanup(func() { os.Stdout, os.Stdin = stdout, stdin })

	fn()

	_ = write.Close()
	out, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}

// TestRunWithConnector exercises the sequence every subcommand shares: flags,
// table, builder, build, sort, filter and print. It was at 0% coverage while
// carrying all thirteen commands.
func TestRunWithConnector(t *testing.T) {
	tests := []struct {
		name        string
		flags       []string
		wantOrder   []string
		wantMissing []string
	}{
		{
			// the fake clientset hands objects back sorted by name, so this
			// asserts every row is rendered rather than a particular order
			name:      "every pod and container is rendered",
			wantOrder: []string{"alpha-pod", "zulu-pod"},
		},
		{
			name:      "--sort orders by the column",
			flags:     []string{"--sort", "PODNAME"},
			wantOrder: []string{"alpha-pod", "zulu-pod"},
		},
		{
			name:      "--sort with ! reverses",
			flags:     []string{"--sort", "!PODNAME"},
			wantOrder: []string{"zulu-pod", "alpha-pod"},
		},
		{
			name:        "--match drops the rows that do not match",
			flags:       []string{"--match", "IMAGE=*istio*"},
			wantOrder:   []string{"proxyv2"},
			wantMissing: []string{"alpha-pod"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := runTestCommand(t, "image", test.flags...)

			pods := runTestPods()
			connect := &Connector{clientSet: fake.NewClientset(pods[0], pods[1])}
			connect.SetNamespace("team-a")

			var err error
			out := captureStdout(t, func() {
				err = runWithConnector(cmd, connect, nil, subCommand{
					loop:               &image{},
					loopSpec:           true,
					showInitContainers: true,
				})
			})
			if err != nil {
				t.Fatalf("runWithConnector: %v", err)
			}

			if !strings.Contains(out, "PODNAME") {
				t.Fatalf("no header in the output:\n%s", out)
			}

			at := -1
			for _, want := range test.wantOrder {
				found := strings.Index(out, want)
				if found < 0 {
					t.Fatalf("%q missing from the output:\n%s", want, out)
				}
				if found < at {
					t.Errorf("%q appears before the row that should precede it", want)
				}
				at = found
			}
			for _, missing := range test.wantMissing {
				if strings.Contains(out, missing) {
					t.Errorf("%q should have been filtered out:\n%s", missing, out)
				}
			}
		})
	}
}

// TestRunWithConnectorReportsBuildError checks the error from Build reaches the
// caller. Twelve commands did this and one did not, which is how an api failure
// used to print an empty table and exit 0.
func TestRunWithConnectorReportsBuildError(t *testing.T) {
	cmd := runTestCommand(t, "image")
	connect := &Connector{clientSet: fake.NewClientset()} // no pods at all
	connect.SetNamespace("team-a")

	var err error
	captureStdout(t, func() {
		err = runWithConnector(cmd, connect, nil, subCommand{loop: &image{}, loopSpec: true})
	})

	if err == nil {
		t.Fatal("want an error when there are no pods to show")
	}
}
