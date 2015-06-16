#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

for i in `seq 1 20`
do
echo -e "/n/n/nCHAO==================================================/n/n/n"
echo -e "CHAO: i=$i"
KUBE_TEST_API_VERSIONS=v1 KUBE_INTEGRATION_TEST_MAX_CONCURRENCY=4 LOG_LEVEL=4 ./hack/test-integration.sh
KUBE_TEST_API_VERSIONS=v1beta3 KUBE_INTEGRATION_TEST_MAX_CONCURRENCY=4 LOG_LEVEL=4 ./hack/test-integration.sh
echo "After test"
done
