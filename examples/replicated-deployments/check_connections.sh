#!/bin/bash

declare -A main_cluster_connections  # Store connections from the main cluster

# Function to extract connections
extract_connections() {
    local kubeconfig=$1
    kubectl get connections --kubeconfig="$kubeconfig" -A -o jsonpath="{range .items[*]}{.metadata.name}{': '}{.spec.remoteCluster.identity.clusterID}{' ('}{.spec.remoteCluster.identity.clusterName}{')\n'}{end}"
}

# Get connections from the main cluster
echo "🔎 Fetching connections from the MAIN cluster ($KUBECONFIG)..."
while IFS= read -r line; do
    connection_name=$(echo "$line" | cut -d':' -f1 | xargs)
    remote_cluster_info=$(echo "$line" | cut -d':' -f2 | xargs)

    # Store connection in an associative array
    if [[ -n "$remote_cluster_info" ]]; then
        main_cluster_connections["$remote_cluster_info"]="$connection_name"
    fi
done < <(extract_connections "$KUBECONFIG")

# Now check the secondary clusters
secondary_clusters=("$KUBECONFIG_EUROPE_MILAN_EDGE" "$KUBECONFIG_EUROPE_ROME_EDGE")

for cluster in "${secondary_clusters[@]}"; do
    echo "🔎 Checking connections for secondary cluster: $cluster"

    while IFS= read -r line; do
        connection_name=$(echo "$line" | cut -d':' -f1 | xargs)
        remote_cluster_info=$(echo "$line" | cut -d':' -f2 | xargs)

        # Skip empty cluster IDs
        if [[ -z "$remote_cluster_info" ]]; then
            continue
        fi

        # Check if this remote cluster ID exists in the main cluster
        if [[ -n "${main_cluster_connections[$remote_cluster_info]}" ]]; then
            echo "  ⚠️ WARNING: Duplicate connection detected!"
            echo "     - Remote Cluster: $remote_cluster_info"
            echo "     - Connection exists in MAIN cluster ($KUBECONFIG) via: ${main_cluster_connections[$remote_cluster_info]}"
            echo "     - Also found in secondary cluster ($cluster) via: $connection_name"
        fi
    done < <(extract_connections "$cluster")

    echo ""
done

