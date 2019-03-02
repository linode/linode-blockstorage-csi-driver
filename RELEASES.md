## Release Steps and Notes

```sh
VERSION=v0.1.0
```

```sh
make test
golangci-lint run
hack/release-yaml.sh $VERSION
vi CHANGELOG.md
git add CHANGELOG.md pkg/linode-bs/deploy/releases/linode-blockstorage-csi-driver-$VERSION.yaml
git commit -am '$VERSION release'
git tag -s $VERSION # include changelog text, install instructions, help instructions, etc
make IMAGE_VERSION=$VERSION push
docker tag linode/linode-blockstorage-csi-driver:$VERSION linode/linode-blockstorage-csi-driver:canary
git push  linode/linode-blockstorage-csi-driver:canary
docker  push  linode/linode-blockstorage-csi-driver:canary
```
