# TODO remove?

# whereis ab

# capture output
output=$(curl localhost:1025/session/start -H 'Content-Type: application/json' --data "{\"username\":\"test\",\"password\":\"test\",\"key\":\"0000000000000000000000000000000000000000000000000000000000000000\"}")
port=

# {"port":6000,"token":"2183f89d-71f4-4a12-a716-ab7afb8eddfa","code":"ZLID"}
regex="^{\"port\":(.*),\"token\":\"(.*)\",\"code\":\"(.*)\"$"
if [[ $output =~ $regex ]]
then
    port=${BASH_REMATCH[1]}
    token=${BASH_REMATCH[2]}
else
    echo "$output doesn't match" >&2 # this could get noisy if there are a lot of non-matching files
fi

echo -n -e '\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF' >/dev/udp/localhost/$port

curl localhost:1025/session/stop -H 'Content-Type: application/json' --data "{\"port\":${port},\"token\":\"${token}\"}"
