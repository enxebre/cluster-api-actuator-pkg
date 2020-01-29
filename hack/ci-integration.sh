#!/bin/sh

#go test -timeout 90m \
#  -v ./pkg/e2e \
#  -kubeconfig ${KUBECONFIG:-~/.kube/config} \
#  -args -v 5 -logtostderr \
#  "$@"

# go run ./vendor/github.com/onsi/ginkgo/ginkgo -p -stream -v -failFast ./pkg/e2e/ -- --alsologtostderr -v 4
go run ./vendor/github.com/onsi/ginkgo/ginkgo \
    -timeout 90m \
    -p -stream \
    -v \
    -failFast \
    "$@" \
    ./pkg/e2e/ -- --alsologtostderr -v 4 -kubeconfig ${KUBECONFIG:-~/.kube/config}