#!/bin/sh
CONFIG=${CONFIG:-/etc/pipeline/pipeline.conf}

export KEY=${KEY:-/etc/pipeline/pipeline_key}
[ -f "$KEY" ] || openssl genrsa -out $KEY 4096

if [ -f "$REPLAY" ]; then
	export REPLAY_PIPE=/tmp/replay.pipe
	rm -f $REPLAY_PIPE
	mkfifo $REPLAY_PIPE
	( echo Replaying dataset...; xzcat $REPLAY > $REPLAY_PIPE; rm -f $REPLAY_PIPE; echo Replay completed. ) &
fi

if [ ! -f "$CONFIG" ]; then
	oldsection=
	env | grep ^PIPELINE_ | cut -d_ -f2- | sort | while read line; do
		section=$(echo "$line" | cut -d_ -f1)
		option=$(echo "$line" | cut -d_ -f2-)
		key=$(echo "$option" | cut -d= -f1)
		value=$(echo "$option" | cut -d= -f2-)

		if [ "$key" == "_password" -o "$key" == "_secret" ]; then
			[ "$key" == "_password" ] && password=$value || password=$(cat $value)
			key=password
			value=

			for pwline in $(echo -n ${password} | openssl pkeyutl -encrypt -inkey $KEY -pkeyopt rsa_padding_mode:oaep \
					-pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256 | openssl enc -base64); do
    			value=$value$pwline
        	done
		fi

		[ "$section" != "$oldsection" ] && echo -e "\n[$section]" >> $CONFIG
		echo "$key = $value" >> $CONFIG

		oldsection=$section
	done
fi

exec /bin/pipeline $DEBUG --config=$CONFIG --pem=$KEY --log=