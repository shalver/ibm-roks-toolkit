package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openshift/ibm-roks-toolkit/pkg/cmd/cpoperator"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/clusteroperator"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/clusterversion"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/cmca"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/infrastatus"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/kubeletservingca"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/openshiftapiserver"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/openshiftapiservermonitor"
	"github.com/openshift/ibm-roks-toolkit/pkg/controllers/openshiftcontrollermanager"
)

const (
	defaultReleaseVersion    = "0.0.1-snapshot"
	defaultKubernetesVersion = "0.0.1-snapshot-kubernetes"
)

func main() {
	log.SetLogger(zap.New(zap.UseDevMode(true)))
	setupLog := ctrl.Log.WithName("setup")
	if err := newControlPlaneOperatorCommand().Execute(); err != nil {
		setupLog.Error(err, "Operator failed")
	}
}

var controllerFuncs = map[string]cpoperator.ControllerSetupFunc{
	"controller-manager-ca":        cmca.Setup,
	"cluster-operator":             clusteroperator.Setup,
	"cluster-version":              clusterversion.Setup,
	"kubelet-serving-ca":           kubeletservingca.Setup,
	"openshift-apiserver":          openshiftapiserver.Setup,
	"openshift-controller-manager": openshiftcontrollermanager.Setup,
	"openshift-apiserver-monitor":  openshiftapiservermonitor.Setup,
	"infrastatus":                  infrastatus.Setup,
}

type ControlPlaneOperator struct {
	// Namespace is the namespace on the management cluster where the control plane components run.
	Namespace string

	// TargetKubeconfig is a kubeconfig to access the target cluster.
	TargetKubeconfig string

	// InitialCAFile is a file containing the initial contents of the Kube controller manager CA.
	InitialCAFile string

	// Controllers is the list of controllers that the operator should start
	Controllers []string

	// ReleaseVersion is the OpenShift version for the release
	ReleaseVersion string

	// KubernetesVersion is the kubernetes version included in the release
	KubernetesVersion string

	initialCA []byte
}

func newControlPlaneOperatorCommand() *cobra.Command {
	cpo := newControlPlaneOperator()
	cmd := &cobra.Command{
		Use:   "control-plane-operator",
		Short: "The Hypershift Control Plane Operator contains a set of controllers that manage an OpenShift hosted control plane.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cpo.Validate(); err != nil {
				return err
			}
			if err := cpo.Complete(); err != nil {
				return err
			}
			return cpo.Run(ctrl.SetupSignalHandler())
		},
	}
	flags := cmd.Flags()
	flags.AddGoFlagSet(flag.CommandLine)
	flags.StringVar(&cpo.Namespace, "namespace", cpo.Namespace, "Namespace for control plane components on management cluster")
	flags.StringVar(&cpo.TargetKubeconfig, "target-kubeconfig", cpo.TargetKubeconfig, "Kubeconfig for target cluster")
	flags.StringVar(&cpo.InitialCAFile, "initial-ca-file", cpo.InitialCAFile, "Path to controller manager initial CA file")
	flags.StringSliceVar(&cpo.Controllers, "controllers", cpo.Controllers, "Controllers to run with this operator")
	return cmd
}

func newControlPlaneOperator() *ControlPlaneOperator {
	return &ControlPlaneOperator{
		Controllers: allControllers(),
	}
}

func allControllers() []string {
	controllers := make([]string, 0, len(controllerFuncs))
	for name := range controllerFuncs {
		controllers = append(controllers, name)
	}
	return controllers
}

func (o *ControlPlaneOperator) Validate() error {
	if len(o.Controllers) == 0 {
		return fmt.Errorf("at least one controller is required")
	}
	if len(o.Namespace) == 0 {
		return fmt.Errorf("the namespace for control plane components is required")
	}
	return nil
}

func (o *ControlPlaneOperator) Complete() error {
	var err error
	if len(o.InitialCAFile) > 0 {
		o.initialCA, err = ioutil.ReadFile(o.InitialCAFile)
		if err != nil {
			return err
		}
	}
	o.ReleaseVersion = os.Getenv("OPENSHIFT_RELEASE_VERSION")
	if o.ReleaseVersion == "" {
		o.ReleaseVersion = defaultReleaseVersion
	}
	o.KubernetesVersion = os.Getenv("KUBERNETES_VERSION")
	if o.KubernetesVersion == "" {
		o.KubernetesVersion = defaultKubernetesVersion
	}
	return nil
}

func (o *ControlPlaneOperator) Run(ctx context.Context) error {
	versions := map[string]string{
		"release":    o.ReleaseVersion,
		"kubernetes": o.KubernetesVersion,
	}
	cfg := cpoperator.NewControlPlaneOperatorConfig(
		o.TargetKubeconfig,
		o.Namespace,
		o.initialCA,
		versions,
		o.Controllers,
		controllerFuncs,
	)
	return cfg.Start(ctx)
}
