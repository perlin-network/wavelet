#!/bin/sh

HOST=$(ec2metadata --public-ipv4)

cat <<EOF > /opt/wavelet/wallet.txt
${private_key}
EOF

truncate -s -1 /opt/wavelet/wallet.txt
chown wavelet:wavelet /opt/wavelet/wallet.txt

cat <<EOF > /etc/systemd/system/wavelet.service
[Unit]
Description=Wavelet Node
After=network.target
StartLimitIntervalSec=0

[Service]
Type=simple
Restart=always
RestartSec=1
User=wavelet
Group=wavelet
ExecStart=/opt/wavelet/wavelet --host $HOST --port ${rpc_port} --api.port ${api_port} --wallet /opt/wavelet/wallet.txt -db /opt/wavelet/db ${peer_address}

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable wavelet.service
systemctl start wavelet.service

