/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	accountv1alpha1 "github.com/platform-mesh/account-operator/api/v1alpha1"
	platformmeshcontext "github.com/platform-mesh/golang-commons/config"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/terminal-controller-manager/api/v1alpha1"
	"github.com/platform-mesh/terminal-controller-manager/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	ctrl "sigs.k8s.io/controller-runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	kcpapisv1alpha1 "github.com/kcp-dev/sdk/apis/apis/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var (
	scheme      = runtime.NewScheme()
	operatorCfg config.OperatorConfig
	defaultCfg  *platformmeshcontext.CommonServiceConfig
	v           *viper.Viper
	log         *logger.Logger
)

var rootCmd = &cobra.Command{
	Use:   "terminal-controller-manager",
	Short: "operator to reconcile Terminals",
}

func init() {
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(kcpapisv1alpha1.AddToScheme(scheme))
	utilruntime.Must(accountv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	//+kubebuilder:scaffold:scheme

	rootCmd.AddCommand(operatorCmd)

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
