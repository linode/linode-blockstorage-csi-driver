# Release History

## [v0.1.3](https://github.com/linode/linode-cloud-controller-manager/compare/v0.1.0..v0.1.3)

### Features

* Support arbitrary root CAs

## Enhancements

* Update linodego to 0.10.0 (prepare to support 8+ volumes per VM)

## v0.1.0 - March 2nd 2019

* per the CSI spec, fulfill volume requests with required\_size under 10GB by extending them to 10GB (the Linode minimum), unless that is over the limit size
* added a storage class of `linode-block-storage-retain`, with a default reclaim policy of `Retain` (to avoid deletion of the Block Storage Volume data)

## v0.0.3 - Dec 5th 2018

* Fixed mangling of hyphens in k8s stored volume keys (from prefixes, which affected mount)

## v0.0.2 - Dec 4th 2018

* CSI 1.0.0 Rewrite, now requiring Kubernetes 1.13+
* Added `--bs-prefix` driver argument for prefixing created volume labels

## v0.0.1 - Aug 14th 2018

* Work-In-Progress
