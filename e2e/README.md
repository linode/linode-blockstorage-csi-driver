## How to use these End-to-end (e2e) tests

### Building

Add the following environment variables to your shell rc

```
export LINODE_API_TOKEN=<your linode API token>

export GOPATH=$HOME/go
export PATH=$HOME/go/bin:$PATH
export GO111MODULE=on 
```

If you need a Linode API token visit this page:
https://cloud.linode.com/profile/tokens

Then, `go get` this repo
`go get github.com/linode/linode-blockstorage-csi-driver`

That may fail, if it does, navigate to the directory that was created and run
`go mod tidy`:

```
cd ~/go/src/github.com/linode/linode-blockstorage-csi-driver
go mod tidy
```

Then, use the makefile in the directory above this directory to build the CSI
(this is to download goimports)

```
cd $GOPATH/src/github.com/linode/linode-blockstorage-csi-driver
make build
```

### Running

Build a docker image passing the `IMAGE_TAG` argument to the make target
so a custom tag is applied. Then push the image to a public repository.

> You can use any public repository that you have access to. The tags used below are just examples

```
make docker-build IMAGE_TAG=ghcr.io/avestuk/linode-blockstorage-csi-driver:test-e2e
make docker-push IMAGE_TAG=ghcr.io/avestuk/linode-blockstorage-csi-driver:test-e2e
```

Finally run the tests passing the name of the image and the tag so that your custom image is used.
```
make test IMAGE_NAME=ghcr.io/avestuk/linode-blockstorage-csi-driver IMAGE_TAG=test-e2e
```

When running the e2e tests, a couple of options can be passed to the test
through the `$SUITE_ARGS` environment variable to modify its behavior: 

 - If the Linode API extra debugging logs are desired, simply use the
   `--linode-debug` in the list of suite arguments. NOTE: This will also
   print out the Linode API token in the logs, since they will be part of the
   requests being logged.

 - Similarly, the Linode API base URL can be changed from
   `https://api.linode.com` with the `--linode-url` flag.

#### Creating a new cluster

By default the tests use $HOME/.ssh/id\_rsa.pub as the public key used to
provision the cluster, so it needs to be added to your agent.

```
ssh-add $HOME/.ssh/id_rsa
```

The cluster created will need an expected Kubernetes version, which is defined
by exporting the following environment variable:

```
export K8S_VERSION=<the version in vMM.mm.pp format>
```

Finally, run the tests in the e2e directory:
```
cd e2e
make test
```

To save time on multiple runs by allowing the cluster to remain, export the
`$SUITE_ARGS` and ensure that the `--reuse` flag is set.

#### Using an existing cluster

If using an existing cluster, ensure that the cluster's kubeconfig is available
as a file in your filesystem. Then supply the following flags through the
`$SUITE_ARGS` environment variable to have the e2e use the cluster:

```
export SUITE_ARGS="--use-existing --kubeconfig=<path to kubeconfig>"
```

