export GO111MODULE=on

$(GOPATH)/bin/ginkgo:
	GO111MODULE=off go get -u github.com/onsi/ginkgo/ginkgo

ginkgo_test: $(GOPATH)/bin/ginkgo
	go vet ./
	@if [ -z "${LINODE_API_TOKEN}" ]; then\
		echo "Skipping Test, LINODE_API_TOKEN is not set";\
	else \
		go list -m; \
		ginkgo -r --v --progress --trace --cover -- --v=3; \
	fi

install-terraform:
	sudo apt-get install wget unzip
	wget https://releases.hashicorp.com/terraform/0.11.13/terraform_0.11.13_linux_amd64.zip
	unzip terraform_0.11.13_linux_amd64.zip
	sudo mv terraform /usr/local/bin/