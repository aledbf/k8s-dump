# k8s-dump
Dump objects from a running Kubernetes cluster

**Build:** run `go build`

**Options:**
```
./dump --help
      --alsologtostderr                  log to standard error as well as files
      --apiserver-host string            The address of the Kubernetes Apiserver to connect to in the format of protocol://address:port, e.g., http://localhost:8080. If not specified, the assumption is that the binary runs inside a Kubernetes cluster and local discovery is attempted.
      --kubeconfig string                Path to kubeconfig file with authorization and master location information.
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files
      --namespace string                 Only dump the contents of a particular namespace.
      --output string                    Directory where the dump files should be created.
      --skip-types stringSlice           Types to skip in the dump.  (default [serviceaccount])
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
      -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

In a terminal open a proxy connection to the cluster using kubectl:
```
kubectl proxy --port=8080
Starting to serve on 127.0.0.1:8080
```

**Running the command:**
```
./dump \
    --output=$PWD/out \
    --apiserver-host=http://127.0.0.1:8080

```

will dump all the object types (excluding service accounts) in the directory `--output` creating one file per namespace.
Each

```
./dump --output=/Users/aledbf/go/src/k8s.io/dump/out --apiserver-host=http://127.0.0.1:8080
I0208 21:00:09.493695   64091 main.go:116] Creating API server client for http://127.0.0.1:8080
I0208 21:00:09.883821   64091 main.go:146] Dumping cluster objects...
I0208 21:00:09.883845   64091 main.go:218] 	dumping namespace xxxxxxx
I0208 21:00:13.830584   64091 main.go:218] 	dumping namespace xxxxxxx
...
I0208 21:00:19.830584   64091 main.go:158] 	done.
```

Example of a file:
````

# errors:

# there is no objects of type statefulsets in namespace xxxxxx
# there is no objects of type storageclasses in namespace xxxxxx
# there is no objects of type persistentvolumes in namespace xxxxxx
# there is no objects of type podsecuritypolicies in namespace xxxxxx
# there is no objects of type thirdpartyresources in namespace xxxxxx

# namespace
apiVersion: v1
kind: Namespace
metadata:
  name: xxxxxx

---

# deployments

......
---

# endpoints

......
---

# replicasets

......
---

# secrets

......
---

# services

......
---
```
