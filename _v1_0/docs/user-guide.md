# Kubernetes User Guide

The user guide is intended for anyone who wants to run programs and services
on an existing Kubernetes cluster.  Setup and administration of a
Kubernetes cluster is described in the [Cluster Admin Guide](cluster-admin-guide.html).
The [Developer Guide](developer-guide.html) is for anyone wanting to either write code which directly accesses the
kubernetes API, or to contribute directly to the kubernetes project.

## Primary concepts

* **Overview** ([overview.html](overview.html)): A brief overview
  of Kubernetes concepts. 

* **Nodes** ([node.html](node.html)): A node is a worker machine in Kubernetes.

* **Pods** ([pods.html](pods.html)): A pod is a tightly-coupled group of containers
  with shared volumes.

* **The Life of a Pod** ([pod-states.html](pod-states.html)):
  Covers the intersection of pod states, the PodStatus type, the life-cycle
  of a pod, events, restart policies, and replication controllers.

* **Replication Controllers** ([replication-controller.html](replication-controller.html)):
  A replication controller ensures that a specified number of pod "replicas" are 
  running at any one time.

* **Services** ([services.html](services.html)): A Kubernetes service is an abstraction 
  which defines a logical set of pods and a policy by which to access them.

* **Volumes** ([volumes.html](volumes.html)): A Volume is a directory, possibly with some 
  data in it, which is accessible to a Container.

* **Labels** ([labels.html](labels.html)): Labels are key/value pairs that are 
  attached to objects, such as pods. Labels can be used to organize and to 
  select subsets of objects. 

* **Secrets** ([secrets.html](secrets.html)): A Secret stores sensitive data
  (e.g. ssh keys, passwords) separately from the Pods that use them, protecting
  the sensitive data from proliferation by tools that process pods.

* **Accessing the API and other cluster services via a Proxy** [accessing-the-cluster.md](../docs/accessing-the-cluster.html)

* **API Overview** ([api.html](api.html)): Pointers to API documentation on various topics
  and explanation of Kubernetes's approaches to API changes and API versioning.

* **Kubernetes Web Interface** ([ui.html](ui.html)): Accessing the Kubernetes
  web user interface.

* **Kubectl Command Line Interface** ([kubectl.html](kubectl.html)):
  The `kubectl` command line reference.

* **Sharing Cluster Access** ([sharing-clusters.html](sharing-clusters.html)):
  How to share client credentials for a kubernetes cluster.

* **Roadmap** ([roadmap.html](roadmap.html)): The set of supported use cases, features,
  docs, and patterns that are required before Kubernetes 1.0.

* **Glossary** ([glossary.html](glossary.html)): Terms and concepts.

## Further reading
<!--- make sure all documents from the docs directory are linked somewhere.
This one-liner (execute in docs/ dir) prints unlinked documents (only from this
dir - no recursion):
for i in *.md; do grep -r $i . | grep -v "^\./$i" > /dev/null; rv=$?; if [[ $rv -ne 0 ]]; then echo $i; fi; done
-->

* **Annotations** ([annotations.html](annotations.html)): Attaching
  arbitrary non-identifying metadata.

* **Downward API** ([downward_api.html](downward_api.html)): Accessing system
  configuration from a pod without accessing Kubernetes API (see also
  [container-environment.md](container-environment.html)).

* **Kubernetes Container Environment** ([container-environment.html](container-environment.html)):
  Describes the environment for Kubelet managed containers on a Kubernetes
  node (see also [downward_api.html](downward_api.html)).

* **DNS Integration with SkyDNS** ([dns.html](dns.html)):
  Resolving a DNS name directly to a Kubernetes service.

* **Identifiers** ([identifiers.html](identifiers.html)): Names and UIDs
  explained.

* **Images** ([images.html](images.html)): Information about container images
  and private registries.

* **Logging** ([logging.html](logging.html)): Pointers to logging info.

* **Namespaces** ([namespaces.html](namespaces.html)): Namespaces help different
  projects, teams, or customers to share a kubernetes cluster.

* **Networking** ([networking.html](networking.html)): Pod networking overview.

* **Services and firewalls** ([services-firewalls.html](services-firewalls.html)): How
  to use firewalls.

* **The Kubernetes Resource Model** ([resources.html](resources.html)):
  Provides resource information such as size, type, and quantity to assist in
  assigning Kubernetes resources appropriately.

* The [API object documentation](http://kubernetes.io/third_party/swagger-ui/).

* Frequently asked questions are answered on this project's [wiki](https://github.com/GoogleCloudPlatform/kubernetes/wiki).



[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/docs/user-guide.html?pixel)]()
