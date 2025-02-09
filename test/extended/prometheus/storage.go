package prometheus

import (
	"context"
	"fmt"

	g "github.com/onsi/ginkgo"
	"github.com/openshift/origin/pkg/test/ginkgo/result"
	exutil "github.com/openshift/origin/test/extended/util"
	helper "github.com/openshift/origin/test/extended/util/prometheus"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	// storage_operation_duration_seconds_bucket with the mount operations that are considered acceptable.
	expectedMountTimeSeconds = 120
	// storage_operation_duration_seconds_bucket with the attach operations that are considered acceptable.
	expectedAttachTimeSeconds = 120
)

var _ = g.Describe("[sig-storage][Late] Metrics", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLIWithoutNamespace("prometheus")

		url, bearerToken string
	)

	g.BeforeEach(func() {
		var ok bool
		url, _, bearerToken, ok = helper.LocatePrometheus(oc)
		if !ok {
			g.Skip("Prometheus could not be located on this cluster, skipping prometheus test")
		}
	})
	g.It("should report short attach times", func() {
		checkOperation(oc, url, bearerToken, "kube-controller-manager", "volume_attach", expectedAttachTimeSeconds)
	})
	g.It("should report short mount times", func() {
		checkOperation(oc, url, bearerToken, "kubelet", "volume_mount", expectedMountTimeSeconds)
	})
})

func checkOperation(oc *exutil.CLI, url string, bearerToken string, component string, name string, threshold int) {
	plugins := []string{"kubernetes.io/azure-disk", "kubernetes.io/aws-ebs", "kubernetes.io/gce-pd", "kubernetes.io/cinder", "kubernetes.io/vsphere-volume"}

	// we only consider series sent since the beginning of the test
	testDuration := exutil.DurationSinceStartInSeconds().String()

	tests := map[string]bool{}
	// Check "[total nr. of ops] - [nr. of ops < threshold] > 0' and expect failure (all ops should be < threshold).
	// Using sum(max(...)) to sum all kubelets / controller-managers
	// Adding a comment to make the failure more readable.
	e2e.Logf("Checking that Operation %s time of plugin %s should be <= %d seconds", name, plugins, threshold)
	queryTemplate := `
# Operation %[4]s time of plugin %[1]s should be < %[2]d seconds
  sum(max_over_time(storage_operation_duration_seconds_bucket{job="%[3]s",le="+Inf",operation_name="%[4]s",volume_plugin="%[1]s"}[%[5]s]))
- sum(max_over_time(storage_operation_duration_seconds_bucket{job="%[3]s",le="%[2]d",operation_name="%[4]s",volume_plugin="%[1]s"}[%[5]s]))
> 0`
	for _, plugin := range plugins {
		query := fmt.Sprintf(queryTemplate, plugin, threshold, component, name, testDuration)
		// Expect failure of the query (the result should be 0, all ops are expected to take < threshold)
		tests[query] = false
	}

	err := helper.RunQueries(context.TODO(), oc.NewPrometheusClient(context.TODO()), tests, oc)
	if err != nil {
		result.Flakef("Operation %s of plugin %s took more than %d seconds: %s", name, plugins, threshold, err)
	}
}
