#!/usr/bin/env bash
# Throwaway spike: stage models in a temporary private bucket, launch a t4g.large that
# soaks them locally and terminates itself, then print its SPIKE_RESULT console lines.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
BUCKET="trachuan-spike-${ACCOUNT}"
M=../../work/models/qwen3-0.6b
FILES=(model_q8w.onnx model_q8w.onnx.data model_q8w_a4.onnx model_q8w_a4.onnx.data model_int8.onnx model_int8.onnx.data tokenizer.json)
CODE=(embedders.py serve.py soak.py golden_questions.json)
case "${1:-launch}" in
launch)
  aws s3api head-bucket --bucket "$BUCKET" 2>/dev/null || aws s3 mb "s3://$BUCKET" --region ap-southeast-1
  for f in "${FILES[@]}"; do aws s3 ls "s3://$BUCKET/m/$f" >/dev/null 2>&1 || aws s3 cp --only-show-errors "$M/$f" "s3://$BUCKET/m/$f"; done
  for f in "${CODE[@]}"; do aws s3 cp --only-show-errors "$f" "s3://$BUCKET/$f"; done
  dl=""
  for f in "${FILES[@]}"; do dl+="curl -sf -o m/$f '$(aws s3 presign "s3://$BUCKET/m/$f" --expires-in 10800)'"$'\n'; done
  for f in "${CODE[@]}"; do dl+="curl -sf -o $f '$(aws s3 presign "s3://$BUCKET/$f" --expires-in 10800)'"$'\n'; done
  printf '%s' "$dl" > ../../work/graviton-fetch.sh
  aws s3 cp --only-show-errors ../../work/graviton-fetch.sh "s3://$BUCKET/fetch.sh"
  fetch="curl -sf '$(aws s3 presign "s3://$BUCKET/fetch.sh" --expires-in 10800)' | bash"
  python3 -c 'import sys; t=open("graviton-userdata.sh.tmpl").read(); open(sys.argv[1],"w").write(t.replace("__DOWNLOADS__", sys.stdin.read()))' ../../work/graviton-userdata.sh <<<"$fetch"
  AMI=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 --query Parameter.Value --output text)
  aws ec2 run-instances --image-id "$AMI" --instance-type t4g.large --count 1 \
    --instance-initiated-shutdown-behavior terminate --metadata-options HttpTokens=required \
    --block-device-mappings 'DeviceName=/dev/xvda,Ebs={VolumeSize=20,VolumeType=gp3,DeleteOnTermination=true}' \
    --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=trachuan-spike}]' \
    --user-data "file://../../work/graviton-userdata.sh" --query 'Instances[0].InstanceId' --output text
  ;;
results)
  aws ec2 get-console-output --instance-id "$2" --latest --output text | grep -aE 'SPIKE_(RESULT|CPU|DONE)' || true
  ;;
cleanup)
  aws s3 rb "s3://$BUCKET" --force
  ;;
esac
