#!/usr/bin/env bash
set -e

declare CLUSTER_NAME
declare IMAGE_NAME
declare SCRIPTPATH
declare KUBECONFIG

CLUSTER_NAME="reaper-test"
IMAGE_NAME="reviewreaper:test"
SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
KUBECONFIG="${SCRIPTPATH}/.kubeconfig"

cd ..

main() {
    deps_check
    run_tests

    delete_cluster
}

deps_check() {
    echo "Checking Docker..."
    if command -v docker >/dev/null; then
        echo "Docker OK..."
    else
        echo "Please install docker."
        exit 1
    fi

    echo "Checking kubectl..."
    if command -v kubectl >/dev/null; then
        echo "kubectl OK..."
    else
        echo "Please install kubectl."
        exit 1
    fi

    echo "Checking kind..."
    if command -v kubectl >/dev/null; then
        echo "kind OK..."
    else
        echo "Please install kind."
        exit 1
    fi

    echo "Checking helm..."
    if command -v helm >/dev/null; then
        echo "helm OK..."
    else
        echo "Please install helm."
        exit 1
    fi
    helm_version=$(helm version --short)
    if [[ "${helm_version}" =~ ^v2\..* ]]; then
        echo 'Error: Helm version 2 is installed. Please upgrade to version 3.' >&2
        exit 1
    elif [[ "${helm_version}" =~ ^v3\..* ]]; then
    echo "Helm version is ${helm_version}. It is fine :)"
    fi


}

run_tests() {
    for file in "${SCRIPTPATH}"/test-cases/*.sh
    do
        rm "${SCRIPTPATH}"/test-config.yaml
        clean_artifacts
        source "$file"
        with_config
        build_code
        bootstrap_and_deploy
        run_test
        delete_cluster
    done
}

clean_artifacts() {
    echo "+++ Clean prev test artifacts..."
    kind delete cluster --name "${CLUSTER_NAME}" > /dev/null 2>&1 || true
    docker rmi "${IMAGE_NAME}" --force
}

build_code() {
    echo "+++ Building ReviewReaper with test config..."
    docker build -t "${IMAGE_NAME}" . -f e2e/Dockerfile.test
}

bootstrap_and_deploy(){
    echo "+++ Creating test cluster..."
    kind create cluster --name "${CLUSTER_NAME}" --kubeconfig "${KUBECONFIG}"

    echo "+++ Loading image into cluster..."
    kind load docker-image "${IMAGE_NAME}" --name "${CLUSTER_NAME}"

    echo "+++ storing kubeconfig in ${KUBECONFIG}"
    kind get kubeconfig --name reaper-test > "${KUBECONFIG}"

    echo "+++ Run export KUBECONFIG=${KUBECONFIG} to access test cluster"

    export KUBECONFIG=${KUBECONFIG}

    echo "+++ Installing reviewReaper by helm-chart..."
    helm install review-reaper ./helm-chart --create-namespace --namespace review-reaper
}

delete_cluster(){
    echo "+++ Deleting cluster..."
    kind delete cluster --name "${CLUSTER_NAME}" > /dev/null 2>&1 || true
    echo "Bye!"
}

main