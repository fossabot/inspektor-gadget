package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/kinvolk/inspektor-gadget/pkg/k8sutil"
)

var execsnoopCmd = &cobra.Command{
	Use:               "execsnoop",
	Short:             "Trace new processes",
	Run:               bccCmd("execsnoop"),
	PersistentPreRunE: doesKubeconfigExist,
}
var opensnoopCmd = &cobra.Command{
	Use:               "opensnoop",
	Short:             "Trace files",
	Run:               bccCmd("opensnoop"),
	PersistentPreRunE: doesKubeconfigExist,
}

func init() {
	commands := []*cobra.Command{execsnoopCmd, opensnoopCmd}
	args := []string{"label", "node", "namespace", "podname"}
	for _, command := range commands {
		for _, arg := range args {
			command.PersistentFlags().String(
				arg,
				"",
				fmt.Sprintf("Kubernetes %s selector", arg))
			viper.BindPFlag(arg, command.PersistentFlags().Lookup(arg))
		}
		rootCmd.AddCommand(command)
	}
}

func bccCmd(subprog string) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		contextLogger := log.WithFields(log.Fields{
			"command": "inspektor-gadget straceback list",
			"args":    args,
		})

		client, err := k8sutil.NewClientset(viper.GetString("kubeconfig"))
		if err != nil {
			contextLogger.Fatalf("Error in creating setting up Kubernetes client: %q", err)
		}

		var listOptions = metaV1.ListOptions{
			LabelSelector: labels.Everything().String(),
			FieldSelector: fields.Everything().String(),
		}

		nodes, err := client.CoreV1().Nodes().List(listOptions)
		if err != nil {
			contextLogger.Fatalf("Error in listing nodes: %q", err)
		}

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		tmpId := time.Now().Format("20060102150405")

		for _, node := range nodes.Items {
			if viper.GetString("node") != "" && node.Name != viper.GetString("node") {
				continue
			}
			err := execPodQuickStart(client, node.Name,
			                         fmt.Sprintf("sh -c \"echo \\$\\$ > /run/%s.pid && exec /opt/bcck8s/%s-edge --label '%q' --namespace '%q' --podname '%q' \" || true",
			                                     tmpId, subprog, viper.GetString("label"), viper.GetString("namespace"), viper.GetString("podname") ))
			if err != "" {
				fmt.Printf("Error in running command: %q\n", err)
			}
		}

		<-sigs
		fmt.Printf("Interrupted!\n")
		for _, node := range nodes.Items {
			if viper.GetString("node") != "" && node.Name != viper.GetString("node") {
				continue
			}
			err := execPodQuickStart(client, node.Name,
			                         fmt.Sprintf("sh -c \"touch /run/%s.pid; kill -9 \\$(cat /run/%s.pid); rm /run/%s.pid\"",
			                         tmpId, tmpId, tmpId))
			if err != "" {
				fmt.Printf("Error in running command: %q\n", err)
			}
		}
	}
}