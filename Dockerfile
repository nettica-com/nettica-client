FROM alpine

RUN apk add wireguard-tools
RUN apk add iptables
RUN mkdir -p /etc/nettica
COPY nettica-client /usr/bin
COPY wg-hack/wg-quick /usr/bin
RUN chmod +x /usr/bin/wg-quick
USER root
CMD ["/usr/bin/nettica-client"]
