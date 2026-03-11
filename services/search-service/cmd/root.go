package cmd

import (
	platformmeshcontext "github.com/platform-mesh/golang-commons/config"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/spf13/cobra"

	"github.com/platform-mesh/search/internal/config"
)

var (
	serviceCfg = config.NewServiceConfig()
	defaultCfg *platformmeshcontext.CommonServiceConfig
	log        *logger.Logger
)

var rootCmd = &cobra.Command{
	Use:   "search-service",
	Short: "Platform Mesh search service",
}

func init() {
	rootCmd.AddCommand(serverCmd)

	defaultCfg = platformmeshcontext.NewDefaultConfig()
	defaultCfg.AddFlags(rootCmd.PersistentFlags())
	serviceCfg.AddFlags(serverCmd.Flags())

	cobra.OnInitialize(initLog)
}

func initLog() {
	lCfg := logger.DefaultConfig()
	lCfg.Level = defaultCfg.Log.Level
	lCfg.NoJSON = defaultCfg.Log.NoJson

	var err error
	log, err = logger.New(lCfg)
	if err != nil {
		panic(err)
	}
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}
