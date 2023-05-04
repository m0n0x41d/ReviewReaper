#!/usr/bin/env/bash

with_config() {
cat << EOF > "${SCRIPTPATH}"/test-config.yaml
deletion_name_regexp: feature

retention:
  days: 1
  hours: 1

deletion_batch_size: 3
deletion_nap_seconds: 10

annotation_key: delete_after

uninstall_releases: true

postpone_deletion_if_active: true

deletion_windows:
  not_before: "20:00"
  not_after:  "22:00"
EOF
}

run_test() {
  echo "Spell some bash\helm\kubectl tests here if you have nothing to do with your life"
  sleep infinity
}