#!/usr/bin/env/bash

with_config() {
cat << EOF > "${SCRIPTPATH}"/test-config.yaml
NsNameDeletionRegexp: feature

retention:
  days: 1
  hours: 1

DeletionBatchSize: 3
DeletionNapSeconds: 10

AnnotationKey: delete_after

IsUninstallReleases: true

PostoneNsDeletionByHelmDeploy: true

DeletionWindow:
  NotBefore: "20:00"
  NotAfter:  "22:00"
EOF
}

run_test() {
  echo "Spell some bash\helm\kubectl tests here if you have nothing to do with your life"
  sleep infinity
}