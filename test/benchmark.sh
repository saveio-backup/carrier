#!/usr/bin/env bash

# 测试porter的最大连接数: 6008端口
test_porter_main_server_max_connection(){
    PORTER_IP="192.168.1.115"
    PORTER_PORT="6008"
    PROTOCOL="quic"

    CARRIER_ENV_IP="192.168.1.115"
    CARRIER_START_PORT=9000
    CARRIERS_NUM=3000
    ENABLE_PROXY="true"
    if []; then
        echo ""
    fi
    for (( port = $CARRIER_START_PORT; port < ${CARRIER_START_PORT}+${CARRIERS_NUM}; port++ )); do
        ./carrier -port=${port} -protocol=${PROTOCOL} -host=${CARRIER_ENV_IP} -enableProxy=${ENABLE_PROXY} \
        -proxy=$PROTOCOL://$PORTER_IP:$PORTER_PORT >> /dev/null &
    done
}

#测试一个节点通过porter最多可以挂载多少个连接数
test_remote_c2c_by_porter() {
    CONNECTION_NUM=10
    BEGIN_PORT=5105
    REMOTE_ADDR=10.0.1.128
    REMOTE_PORT=63936
    LOCAL_HOST=192.168.1.115
    for (( index = 0; index < ${CONNECTION_NUM}; index++ )); do
        echo "=============================="$index
        nohup ./carrier -port=$((${BEGIN_PORT}+${index})) -protocol=tcp -host=${LOCAL_HOST} -enableProxy=true -proxy=tcp://10.0.1.128:6008 -peers=tcp://${REMOTE_ADDR}:${REMOTE_PORT} &
    done
}

case "$1" in
max_conn)
    test_porter_main_server_max_connection
;;
run)
    test_remote_c2c_by_porter
;;
*)
echo "please enter invalid command..."
esac
