package cmd

import (
	platformmeshcontext "github.com/platform-mesh/golang-commons/config"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	migrationv1alpha1 "github.com/platform-mesh/kcp-migration-operator/api/v1alpha1"
	"github.com/platform-mesh/kcp-migration-operator/internal/config"
)

var (
	scheme      = runtime.NewScheme()
	operatorCfg config.OperatorConfig
	defaultCfg  *platformmeshcontext.CommonServiceConfig
	v           *viper.Viper
	log         *logger.Logger
)

var rootCmd = &cobra.Command{
	Use:   "kcp-migration-operator",
	Short: "Operator to migrate Kubernetes resources to KCP workspaces",
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(migrationv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	rootCmd.AddCommand(operatorCmd)
	rootCmd.AddCommand(syncCmd)

	var err error
	v, defaultCfg, err = platformmeshcontext.NewDefaultConfig(rootCmd)
	if err != nil {
		panic(err)
	}

	err = platformmeshcontext.BindConfigToFlags(v, operatorCmd, &operatorCfg)
	if err != nil {
		panic(err)
	}

	cobra.OnInitialize(initLog)
}

func initLog() { // coverage-ignore
	logcfg := logger.DefaultConfig()
	logcfg.Level = defaultCfg.Log.Level
	logcfg.NoJSON = defaultCfg.Log.NoJson

	var err error
	log, err = logger.New(logcfg)
	if err != nil {
		panic(err)
	}
	ctrl.SetLogger(log.Logr())
}

func Execute() { // coverage-ignore
	cobra.CheckErr(rootCmd.Execute())
}
