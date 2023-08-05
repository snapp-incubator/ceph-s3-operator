#!/bin/bash

# Source(with modifications): https://github.com/ceph/go-ceph/blob/master/entrypoint.sh

set -e

MICRO_OSD_PATH="/micro-osd.sh"
CEPH_CONF=/tmp/ceph/ceph.conf
MIRROR_STATE=/dev/null

CLI="$(getopt -o h --long test-run:,micro-osd:,wait-for:,ceph-conf:,mirror:,mirror-state:,help -n "${0}" -- "$@")"
eval set -- "${CLI}"
while true ; do
    case "${1}" in
        --micro-osd)
            MICRO_OSD_PATH="${2}"
            shift
            shift
        ;;
        --wait-for)
            WAIT_FILES="${2}"
            shift
            shift
        ;;
        --ceph-conf)
            CEPH_CONF="${2}"
            shift
            shift
        ;;
        --mirror)
            MIRROR_CONF="${2}"
            shift
            shift
        ;;
        --mirror-state)
            MIRROR_STATE="${2}"
            shift
            shift
        ;;
        -h|--help)
            echo "Options:"
            echo "  --test-run=VALUE    Run selected test or ALL, NONE"
            echo "                      ALL is the default"
            echo "  --pause             Sleep forever after tests execute"
            echo "  --micro-osd         Specify path to micro-osd script"
            echo "  --wait-for=FILES    Wait for files before starting tests"
            echo "                      (colon separated, disables micro-osd)"
            echo "  --mirror-state=PATH Path to track state of (rbd) mirroring"
            echo "  --ceph-conf=PATH    Specify path to ceph configuration"
            echo "  --mirror=PATH       Specify path to ceph conf of mirror"
            echo "  -h|--help           Display help text"
            echo ""
            exit 0
        ;;
        --)
            shift
            break
        ;;
        *)
            echo "unknown option ${1}" >&2
            exit 2
        ;;
    esac
done

show() {
    local ret
    echo "*** running:" "$@"
    "$@"
    ret=$?
    if [ ${ret} -ne 0 ] ; then
        echo "*** ERROR: returned ${ret}"
    fi
    return ${ret}
}

wait_for_files() {
    for file in "$@" ; do
        echo -n "*** waiting for $file ..."
        while ! [[ -f $file ]] ; do
            sleep 1
        done
        echo "done"
    done
}

setup_mirroring() {
    mstate="$(cat "${MIRROR_STATE}" 2>/dev/null || true)"
    if [[ "$mstate" = functional ]]; then
        echo "Mirroring already functional"
        return 0
    fi
    echo "Setting up mirroring..."
    local CONF_A=${CEPH_CONF}
    local CONF_B=${MIRROR_CONF}
    ceph -c "$CONF_A" osd pool create rbd 8
    ceph -c "$CONF_B" osd pool create rbd 8
    rbd -c "$CONF_A" pool init
    rbd -c "$CONF_B" pool init
    rbd -c "$CONF_A" mirror pool enable rbd image
    rbd -c "$CONF_B" mirror pool enable rbd image
    token=$(rbd -c "$CONF_A" mirror pool peer bootstrap create --site-name ceph_a rbd)
    echo "bootstrap token: ${token}"
    echo "${token}" | rbd -c "$CONF_B" mirror pool peer bootstrap import --site-name ceph_b rbd -

    echo "enabled" > "${MIRROR_STATE}"
    rbd -c "$CONF_A" rm mirror_test 2>/dev/null || true
    rbd -c "$CONF_B" rm mirror_test 2>/dev/null || true
    (echo "Mirror Test"; dd if=/dev/zero bs=1 count=500K) | rbd -c "$CONF_A" import - mirror_test

    if [[ ${CEPH_VERSION} != nautilus ]]; then
        rbd -c "$CONF_A" mirror image enable mirror_test snapshot
        echo -n "Waiting for mirroring activation..."
        while ! rbd -c "$CONF_A" mirror image status mirror_test \
          | grep -q "state: \+up+replaying" ; do
            sleep 1
        done
        echo "done"
        rbd -c "$CONF_A" mirror image snapshot mirror_test
    else
        rbd -c "$CONF_A" feature enable mirror_test journaling
        rbd -c "$CONF_A" mirror image enable mirror_test
        echo -n "Waiting for mirroring activation..."
        while ! rbd -c "$CONF_B" mirror image status mirror_test \
          | grep -q "state: \+up+replaying" ; do
            sleep 1
        done
        echo "done"
        rbd -c "$CONF_A" mirror image demote mirror_test
        while ! rbd -c "$CONF_B" mirror image status mirror_test \
          | grep -q "state: \+up+stopped" ; do
            sleep 1
        done
        rbd -c "$CONF_B" mirror image status mirror_test
        rbd -c "$CONF_B" mirror image promote mirror_test
        rbd -c "$CONF_B" mirror image disable mirror_test
    fi

    echo -n "Waiting for mirror sync..."
    while ! rbd -c "$CONF_B" export mirror_test - 2>/dev/null | grep -q "Mirror Test" ; do
        sleep 1
    done
    echo "functional" > "${MIRROR_STATE}"
    echo "mirroring functional!"
}

setup-micro-osd() {
    mkdir -p /tmp/ceph
    if ! [[ ${WAIT_FILES} ]]; then
        show "${MICRO_OSD_PATH}" /tmp/ceph
    fi
    export CEPH_CONF

    if [[ ${WAIT_FILES} ]]; then
        # this is less gross looking than any other bash-native split-to-array code
        # shellcheck disable=SC2086
        wait_for_files ${WAIT_FILES//:/ }
    fi
    if [[ ${MIRROR_CONF} ]]; then
      setup_mirroring
      export MIRROR_CONF
    fi
}

setup-micro-osd
echo "run sleep to keep container up"
sleep infinity

# vim: set ts=4 sw=4 sts=4 et:
