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

echo "+++ Clean prev test artifacts..."
kind delete cluster --name "${CLUSTER_NAME}" > /dev/null 2>&1 || true
# docker rmi "${IMAGE_NAME}" --force

echo "+++ Building ReviewReaper with test config..."
docker build -t "${IMAGE_NAME}" . -f e2e/Dockerfile.test

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

sleep 9999999
echo "+++ Deleting cluster..."
kind delete cluster --name "${CLUSTER_NAME}" > /dev/null 2>&1 || true
echo "Bye!"

trap "kill $! 2> /dev/null" EXIT