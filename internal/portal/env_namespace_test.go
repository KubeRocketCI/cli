package portal

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// namespaceReads are the two reads of an environment namespace, for the tests
// of what they share. denied is the message of each for a 403 of the Portal.
var namespaceReads = map[string]struct {
	kind   string
	denied string
	call   func(s *EnvService, deployment, env string) error
}{
	"pods": {
		kind:   "Pod",
		denied: `listing pods in namespace "my-pipeline-dev": permission denied`,
		call: func(s *EnvService, deployment, env string) error {
			_, err := s.Pods(context.Background(), deployment, env)
			return err
		},
	},
	"events": {
		kind:   "Event",
		denied: `listing events in namespace "my-pipeline-dev": permission denied`,
		call: func(s *EnvService, deployment, env string) error {
			_, err := s.Events(context.Background(), deployment, env, EnvEventFilters{})
			return err
		},
	},
}

func TestEnvService_NamespaceReads_PermissionDenied(t *testing.T) {
	t.Parallel()

	for name, read := range namespaceReads {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rec := envWithStage(t, "in-cluster")
			rec.statusByKind = map[string]int{read.kind: http.StatusForbidden}

			svc, closer := newEnvServiceForTest(t, rec)
			defer closer()

			err := read.call(svc, "my-pipeline", "dev")
			if !errors.Is(err, ErrPermissionDenied) || err.Error() != read.denied {
				t.Errorf("error = %v, want ErrPermissionDenied as %q", err, read.denied)
			}
		})
	}
}

// TestEnvService_NamespaceReads_Refusals checks that Pods and Events refuse
// before they read the namespace.
func TestEnvService_NamespaceReads_Refusals(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cluster    string
		namespace  string
		deployment string
		env        string
		sentinel   error
		message    string
	}{
		"unknown deployment": {
			cluster: "in-cluster", namespace: "my-pipeline-dev", deployment: "nope", env: "dev",
			sentinel: ErrDeploymentNotFound, message: ErrDeploymentNotFound.Error(),
		},
		"unknown environment": {
			cluster: "in-cluster", namespace: "my-pipeline-dev", deployment: "my-pipeline", env: "wat",
			sentinel: ErrEnvNotFound, message: ErrEnvNotFound.Error(),
		},
		"environment on another cluster": {
			cluster: "prod-cluster", namespace: "my-pipeline-dev", deployment: "my-pipeline", env: "dev",
			sentinel: ErrRemoteCluster,
			message:  remoteClusterError("my-pipeline", "dev", "prod-cluster").Error(),
		},
		"environment without a namespace": {
			cluster: "in-cluster", deployment: "my-pipeline", env: "dev",
			message: noNamespaceError("my-pipeline", "dev").Error(),
		},
	}

	for readName, read := range namespaceReads {
		for caseName, tc := range cases {
			t.Run(readName+"/"+caseName, func(t *testing.T) {
				t.Parallel()

				rec := envWithStageIn(t, tc.cluster, tc.namespace)

				svc, closer := newEnvServiceForTest(t, rec)
				defer closer()

				err := read.call(svc, tc.deployment, tc.env)
				if err == nil || (tc.sentinel != nil && !errors.Is(err, tc.sentinel)) {
					t.Fatalf("expected %v, got %v", tc.sentinel, err)
				}

				if err.Error() != tc.message {
					t.Errorf("error = %q, want %q", err.Error(), tc.message)
				}

				if calls := callsOfKind(rec, read.kind); len(calls) != 0 {
					t.Errorf("the namespace must not be read after a refusal, got %d %s calls", len(calls), read.kind)
				}
			})
		}
	}
}
