#!/usr/bin/env/bash

with_config() {
cat << EOF > "${SCRIPTPATH}"/test-config.yaml

  deletion_name_regexp: feature

  retention:
    days: 1
    hours: 2

  deletion_batch_size: 1
  deletion_nap_seconds: 1

  annotation_key: delete_after

  uninstall_releases: true

  postpone_deletion_if_active: true

  deletion_windows:
    not_before: "00:00"
    not_after:  "23:59"
EOF
}

run_test() {
  echo "Spell some helm and kubectl shit here"
}