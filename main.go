package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"sync"
	text_template "text/template"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"

	"k8s.io/kubernetes/pkg/api"
	k8s_errors "k8s.io/kubernetes/pkg/api/errors"
	apps "k8s.io/kubernetes/pkg/apis/apps/v1beta1"
	autoscalingapiv1 "k8s.io/kubernetes/pkg/apis/autoscaling/v1"
	batch "k8s.io/kubernetes/pkg/apis/batch/v2alpha1"
	"k8s.io/kubernetes/pkg/apis/extensions"
	storage "k8s.io/kubernetes/pkg/apis/storage/v1beta1"
	client "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	restclient "k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	clientcmdapi "k8s.io/kubernetes/pkg/client/unversioned/clientcmd/api"
	"k8s.io/kubernetes/pkg/kubectl"
	"k8s.io/kubernetes/pkg/runtime"
)

func main() {
	var (
		flags = pflag.NewFlagSet("", pflag.ExitOnError)

		apiserverHost = flags.String("apiserver-host", "", "The address of the Kubernetes Apiserver "+
			"to connect to in the format of protocol://address:port, e.g., "+
			"http://localhost:8080. If not specified, the assumption is that the binary runs inside a "+
			"Kubernetes cluster and local discovery is attempted.")
		kubeConfigFile = flags.String("kubeconfig", "", "Path to kubeconfig file with authorization and master location information.")
		skipTypes      = flags.StringSlice("skip-types", []string{"serviceaccount"}, "Types to skip in the dump. ")
		output         = flags.String("output", "", "Directory where the dump files should be created.")
		namespace      = flags.String("namespace", "", "Only dump the contents of a particular namespace.")
	)

	flags.AddGoFlagSet(flag.CommandLine)
	flags.Parse(os.Args)

	flag.Set("logtostderr", "true")

	kubeClient, err := createApiserverClient(*apiserverHost, *kubeConfigFile)
	if err != nil {
		handleFatalInitError(err)
	}

	dumpCluster(kubeClient, *output, *namespace, *skipTypes)
}

const (
	// High enough QPS to fit all expected use cases. QPS=0 is not set here, because
	// client code is overriding it.
	defaultQPS = 1e6
	// High enough Burst to fit all expected use cases. Burst=0 is not set here, because
	// client code is overriding it.
	defaultBurst = 1e6

	template = `
# errors:
{{ range $i, $v := .notFound }}
# {{ $v }}{{ end }}

# namespace
apiVersion: v1
kind: Namespace
metadata:
  name: {{ .name }}

---

{{ template "iterate" . }}


{{ define "iterate" }}
{{ range $k, $v := .types }}
{{- if ne (len $v.Items) 0 }}
# {{ $k }}
{{ range $item := $v.Items }}
{{ objectToYaml $item }}

---
{{ end }}
{{ end }}
{{- end }}
{{ end }}
`
)

// createApiserverClient creates new Kubernetes Apiserver client. When kubeconfig or apiserverHost param is empty
// the function assumes that it is running inside a Kubernetes cluster and attempts to
// discover the Apiserver. Otherwise, it connects to the Apiserver specified.
//
// apiserverHost param is in the format of protocol://address:port/pathPrefix, e.g.http://localhost:8001.
// kubeConfig location of kubeconfig file
func createApiserverClient(apiserverHost string, kubeConfig string) (*client.Clientset, error) {

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfig},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: apiserverHost}})

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	cfg.QPS = defaultQPS
	cfg.Burst = defaultBurst
	cfg.ContentType = "application/vnd.kubernetes.protobuf"

	glog.Infof("Creating API server client for %s", cfg.Host)

	client, err := client.NewForConfig(cfg)

	if err != nil {
		return nil, err
	}
	return client, nil
}

/**
 * Handles fatal init error that prevents server from doing any work. Prints verbose error
 * message and quits the server.
 */
func handleFatalInitError(err error) {
	glog.Fatalf("Error while initializing connection to Kubernetes apiserver. "+
		"This most likely means that the cluster is misconfigured (e.g., it has "+
		"invalid apiserver certificates or service accounts configuration). Reason: %s\n"+
		"Refer to the troubleshooting guide for more information: "+
		"https://github.com/kubernetes/ingress/blob/master/docs/troubleshooting.md", err)
}

// dump extracts information from a Kubernetes cluster and creates multiple
// files (one per namespace) with the content
func dumpCluster(kubeClient *client.Clientset, output, namespace string, skipTypes []string) {
	nss, err := kubeClient.Namespaces().List(api.ListOptions{})
	if err != nil {
		glog.Fatalf("unexpected error obtaining information about the namespaces: %v", err)
	}

	glog.Infof("Dumping cluster objects...")
	if namespace != "" {
		err := dumpNamespace(kubeClient, namespace, output, skipTypes)
		if err != nil {
			glog.Fatalf("unexpected error obtaining information about the namespaces: %v", err)
		}

		glog.Infof("done")
		os.Exit(0)
	}

	var wg sync.WaitGroup
	for _, ns := range nss.Items {
		if ns.Status.Phase == api.NamespaceTerminating {
			glog.Infof("skiping namespace %v (is being terminated)", ns.Name)
			continue
		}

		wg.Add(1)
		name := ns.Name
		go func() {
			err := dumpNamespace(kubeClient, name, output, skipTypes)
			if err != nil {
				glog.Fatalf("unexpected error dumping namespace (%v) content: %v", name, err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
	glog.Infof("done")
}

func newMappingFactoring() map[string]runtime.Object {
	return map[string]runtime.Object{
		"configmaps":               &api.ConfigMapList{},
		"daemonsets":               &extensions.DaemonSetList{},
		"deployments":              &extensions.DeploymentList{},
		"endpoints":                &api.EndpointsList{},
		"horizontalpodautoscalers": &autoscalingapiv1.HorizontalPodAutoscalerList{},
		"ingresses":                &extensions.IngressList{},
		"jobs":                     &batch.JobList{},
		"limitranges":              &api.LimitRangeList{},
		"networkpolicies":          &extensions.NetworkPolicyList{},
		"persistentvolumeclaims":   &api.PersistentVolumeClaimList{},
		"persistentvolumes":        &api.PersistentVolumeList{},
		"podsecuritypolicies":      &extensions.PodSecurityPolicyList{},
		"podtemplates":             &api.PodTemplateList{},
		"replicasets":              &extensions.ReplicaSetList{},
		"replicationcontrollers":   &api.ReplicationControllerList{},
		"resourcequotas":           &api.ResourceQuotaList{},
		"secrets":                  &api.SecretList{},
		"services":                 &api.ServiceList{},
		"statefulsets":             &apps.StatefulSetList{},
		"storageclasses":           &storage.StorageClassList{},
		"thirdpartyresources":      &extensions.ThirdPartyResourceList{},
	}
}

var (
	regex = regexp.MustCompile("(\\s*)resourceVersion: \"(\\d+)\"")
)

// dumpNamespace extracts information about Kubernetes objects located in a
// particular namespace.
func dumpNamespace(kubeClient *client.Clientset, ns, output string, skipTypes []string) error {
	glog.Infof("\tdumping namespace %v", ns)

	content := make(map[string]interface{})
	data := make(map[string]interface{})
	notFound := []string{}

	t, err := text_template.New("dump").Funcs(text_template.FuncMap{
		"objectToYaml": func(obj runtime.Object) string {
			s, err := marshalYaml(obj)
			if err != nil {
				glog.Errorf("unexpected error converting object to yaml: %v", err)
			}
			return s
		},
	}).Parse(template)

	if err != nil {
		return errors.Wrap(err, "unexpected error parsing template")
	}

	for objectType, result := range newMappingFactoring() {
		if skipType(objectType, skipTypes) {
			glog.Warningf("skipping type %v", objectType)
			continue
		}

		var rc restclient.Interface

		switch objectType {
		case "horizontalpodautoscalers":
			rc = kubeClient.Autoscaling().RESTClient()
		case "jobs":
			rc = kubeClient.Batch().RESTClient()
		case "statefulsets":
			rc = kubeClient.Apps().RESTClient()
		case "storageclasses":
			rc = kubeClient.Storage().RESTClient()
		case "daemonsets", "deployments", "ingresses", "networkpolicies", "podsecuritypolicies", "replicasets", "thirdpartyresources":
			rc = kubeClient.Extensions().RESTClient()
		default:
			rc = kubeClient.Core().RESTClient()
		}

		err = rc.Get().
			Namespace(ns).
			Resource(objectType).
			VersionedParams(&api.ListOptions{}, api.ParameterCodec).
			Do().
			Into(result)

		if err != nil {
			if !k8s_errors.IsNotFound(err) {
				return errors.Wrap(err, "unexpected error querying type")
			}
			notFound = append(notFound, fmt.Sprintf("there is no object of type %v in namespace %v", objectType, ns))
		}

		data[objectType] = result
	}

	content["notFound"] = notFound
	content["name"] = ns
	content["types"] = data

	tmplBuf := new(bytes.Buffer)
	err = t.Execute(tmplBuf, content)
	if err != nil {
		return errors.Wrap(err, "unexpected error populating template")
	}

	path := fmt.Sprintf("%v/%v.yaml", output, ns)
	return ioutil.WriteFile(path, tmplBuf.Bytes(), 0644)
}

// skipType returns true if a slice contains an element with a particular name
func skipType(skip string, names []string) bool {
	for _, name := range names {
		if skip == name {
			return true
		}
	}
	return false
}

// marshalYaml converts an instance of Object interface to a yaml representation
// removing the field resourceVersion
func marshalYaml(obj runtime.Object) (string, error) {
	printer := &kubectl.YAMLPrinter{}
	tmplBuf := new(bytes.Buffer)
	err := printer.PrintObj(obj, tmplBuf)
	if err != nil {
		return "", err
	}

	return regex.ReplaceAllString(tmplBuf.String(), ""), nil
}
