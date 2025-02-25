#!/usr/bin/env bash

set -e

here="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
# shellcheck source=/dev/null
source "$here/../common.sh"

CLUSTER_NAME_DESTINATION3=europe-turin-edge

CLUSTER_LABEL_DESTINATION="topology.liqo.io/type=destination"

KUBECONFIG_DESTINATION3=liqo_kubeconf_europe-turin-edge

LIQO_CLUSTER_CONFIG_YAML3="$here/manifests/cluster4.yaml"

check_requirements

create_cluster "$CLUSTER_NAME_DESTINATION3" "$KUBECONFIG_DESTINATION3" "$LIQO_CLUSTER_CONFIG_YAML3"

install_liqo "$CLUSTER_NAME_DESTINATION3" "$KUBECONFIG_DESTINATION3" "$CLUSTER_LABEL_DESTINATION"
