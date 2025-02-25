FROM kindest/node:v1.30.0
# Install required networking tools
RUN apt-get update && apt-get install -y \
    tcpdump \
    iputils-ping \
    iproute2 \
    net-tools \
    curl \
    wireguard \
    nano \
    && rm -rf /var/lib/apt/lists/*
