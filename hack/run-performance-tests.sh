#!/bin/bash

infomsg() {
  echo "[INFO] ${1}"
}

SETUP_ONLY="false"
TESTS_ONLY="false"
DELETE_ONLY="false"
TEST_NAMESPACES="5"

# process command line args
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    -so|--setup-only)
      SETUP_ONLY="${2}"
      if [ "${SETUP_ONLY}" != "true" -a "${SETUP_ONLY}" != "false" ]; then
        echo "--setup-only option must be one of 'true' or 'false'"
        exit 1
      fi
      shift;shift
      ;;
    -to|--tests-only)
      TESTS_ONLY="${2}"
      if [ "${TESTS_ONLY}" != "true" -a "${TESTS_ONLY}" != "false" ]; then
        echo "--tests-only option must be one of 'true' or 'false'"
        exit 1
      fi
      shift;shift
      ;;
    -do|--delete-only)
      DELETE_ONLY="${2}"
      if [ "${DELETE_ONLY}" != "true" -a "${DELETE_ONLY}" != "false" ]; then
        echo "--delete-only option must be one of 'true' or 'false'"
        exit 1
      fi
      shift;shift
      ;;
    -tn|--test-namespaces)
      TEST_NAMESPACES="${2}"
      if [[ $2 -le 0 || $2 -ge 1000 ]]; then
        echo "--test-namespaces option must be is a valid number between 1 and 1000."
        exit 1
      fi
      shift;shift
      ;;
    -h|--help)
      cat <<HELPMSG
Valid command line arguments:
  -so|--setup-only <true|false>
    If true, only setup the test environment and exit without running the tests.
    Default: false
  -to|--tests-only <true|false>
    If true, only run the tests and skip the setup.
    Default: false
  -do|--delete-only <true|false>
    If true, only delete the test namespaces.
    Default: false
  -tn|--test-namespaces <number>
    Number of test namespaces created before performance run.
    Default: "5"
  -h|--help:
    This message

HELPMSG
      exit 1
      ;;
    *)
      echo "ERROR: Unknown argument [$key]. Aborting."
      exit 1
      ;;
  esac
done

if [ "${SETUP_ONLY}" == "true" -a "${TESTS_ONLY}" == "true" -a "${DELETE_ONLY}" == "true" ]; then
  echo "ERROR: --setup-only --tests-only and --delete-only cannot all be true. Aborting."
  exit 1
fi


# print out our settings for debug purposes
cat <<EOM
=== SETTINGS ===
SETUP_ONLY=$SETUP_ONLY
TESTS_ONLY=$TESTS_ONLY
DELETE_ONLY=$DELETE_ONLY
TEST_NAMESPACES=$TEST_NAMESPACES
=== SETTINGS ===
EOM

set -e

# Determine where this script is and make it the cwd
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
COMMON_PARAMS="${SCRIPT_DIR}/../frontend/cypress/fixtures/perf/commonParams.json"
OUTPUT_DIR=${SCRIPT_DIR}/../frontend/cypress/results
OUTPUT_FILE=${OUTPUT_DIR}/performance.txt

ensureCypressInstalled() {
  cd "${SCRIPT_DIR}"/../frontend
  if ! yarn cypress --help &> /dev/null; then
    echo "cypress binary was not detected in your PATH. Did you install the frontend directory? Before running the frontend tests you must run 'make build-ui'."
    exit 1
  fi
  cd -
}

createNamespaces() {
  ISTIO_INGRESS_IP="$(kubectl get svc istio-ingressgateway -n istio-system -o=jsonpath='{.status.loadBalancer.ingress[0].ip}')"
  # Install demo apps
  "${SCRIPT_DIR}"/istio/install-testing-demos.sh -c "kubectl" -g "${ISTIO_INGRESS_IP}"
  for ((i = 1; i <= $TEST_NAMESPACES; i++)); do
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: perf-test-${i}
  labels:
    kiali.io: perf-test
    istio-injection: enabled
EOF
  jq --arg i "$i" '.allNamespaces += ",perf-test-\($i)"' "$COMMON_PARAMS" > "$COMMON_PARAMS.tmp" && mv "$COMMON_PARAMS.tmp" "$COMMON_PARAMS"
  done
}

deleteNamespaces() {
  kubectl delete --ignore-not-found=true -l kiali.io=perf-test ns
  jq '.allNamespaces = .namespaces' "$COMMON_PARAMS" > "$COMMON_PARAMS.tmp" && mv "$COMMON_PARAMS.tmp" "$COMMON_PARAMS"
}

if [ "${TESTS_ONLY}" != "true" -a "${DELETE_ONLY}" != "true" ]; then
  infomsg "Install test namespaces"
  createNamespaces
fi

if [ "${DELETE_ONLY}" == "true" ]; then
  infomsg "Remove test namespaces"
  deleteNamespaces
  exit 0
fi

export CYPRESS_NUM_TESTS_KEPT_IN_MEMORY=0
# Recorded video is unusable due to low resources in CI: https://github.com/cypress-io/cypress/issues/4722
export CYPRESS_VIDEO=false

if [ "${SETUP_ONLY}" == "true" ]; then
  exit 0
fi

ensureCypressInstalled

cd "${SCRIPT_DIR}"/../frontend
infomsg "Running cypress performance tests"
mkdir "$OUTPUT_DIR"
echo "[Running cypress performance tests for $TEST_NAMESPACES namespaces]" > $OUTPUT_FILE
yarn cypress:run:perf

if [ "${TESTS_ONLY}" != "true" ]; then
  infomsg "Remove test namespaces"
  deleteNamespaces
fi


