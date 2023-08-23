FROM ubuntu:bionic

RUN mkdir -p /etc/nettica
# RUN echo "nameserver 8.8.8.8" > /etc/resolv.conf
RUN apt-get update && apt-get -y install curl gnupg
RUN curl -s -o /etc/apt/sources.list.d/nettica.list https://ppa.nettica.com/nettica.list
RUN curl https://ppa.nettica.com/nettica.gpg | gpg -o /usr/share/keyrings/nettica.gpg --dearmor --batch --yes
RUN apt-get update && apt-get -y install nettica-client wireguard-tools iproute2 inetutils-ping iptables
RUN apt-get install -y apt-utils debconf-utils dialog
RUN echo 'debconf debconf/frontend select Noninteractive' | debconf-set-selections
RUN echo "resolvconf resolvconf/linkify-resolvconf boolean false" | debconf-set-selections
RUN apt-get update
RUN apt-get install -y resolvconf

CMD nettica-client


