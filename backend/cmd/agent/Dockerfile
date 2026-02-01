FROM amazonlinux:latest
ADD KCroot.crt /etc/pki/ca-trust/source/anchors/KCroot.crt
RUN yum install -y unzip git && update-ca-trust && mkdir -p /tmp/.terraform.d/plugin-cache
#ADD credentials.tfrc.json /root/.terraform.d/credentials.tfrc.json
ADD agent /tmp/
ENTRYPOINT ["/tmp/agent"]
