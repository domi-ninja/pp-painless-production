"""Test bucket permissions and persistence using disposable Docker resources."""
import os,subprocess,tempfile,uuid,json,pathlib,time
if os.environ.get('PP_TEST_DOCKER') != '1':
 print('Set PP_TEST_DOCKER=1 to run isolated storage container tests.')
 raise SystemExit(0)
repo=pathlib.Path(__file__).resolve().parents[3];os.chdir(repo); identity='pp-audit-'+uuid.uuid4().hex[:10]; network=identity; container=identity+'-s3'; client_container=identity+'-client'
def run(*args,**kwargs):
 result=subprocess.run(args,capture_output=True,text=True,**kwargs)
 if result.returncode:raise RuntimeError(result.stderr[:3000])
 return result
with tempfile.TemporaryDirectory(prefix='pp-storage-') as tmp:
 file=pathlib.Path(tmp)/'.env.prod'
 run('node','--','examples/convex-website/scripts/bootstrap-env.cjs','site.example.com','--project',identity,'--env-file',str(file))
 values=json.loads(run('node','-e','console.log(JSON.stringify(require(process.argv[1]).readEnv(process.argv[2])))',str(repo/'examples/convex-website/scripts/env-file.cjs'),str(file)).stdout)
 envfile=pathlib.Path(tmp)/'storage.env';envfile.write_text(''.join(k+'='+v+'\n' for k,v in values.items() if k.startswith(('STORAGE_','S3_STORAGE_'))));envfile.chmod(0o600)
 run('docker','network','create',network)
 try:
  run('docker','run','-d','--name',container,'--network',network,'--network-alias','s3','--env-file',str(envfile),'pp-audit-storage:seaweed')
  for attempt in range(60):
   result=subprocess.run(['docker','exec',container,'python3','/opt/pp/storage.py','health'],capture_output=True,text=True)
   if not result.returncode:break
   if run("docker","inspect",container,"--format","{{.State.Running}}").stdout.strip() != "true":break
   time.sleep(1)
  else:
   logs=run('docker','logs',container).stdout
   for v in values.values():
    if len(v)>10:logs=logs.replace(v,'[redacted]')
   raise RuntimeError('Storage did not become ready: '+logs[-4000:])
  if result.returncode:raise RuntimeError(run('docker','logs',container).stderr[-2500:])
  script='''import os,sys,urllib.request,socket
sys.path.insert(0,"/opt/pp")
import storage
# Use the same restricted account from another container.
import boto3
from botocore.config import Config
s3=boto3.client("s3",endpoint_url="http://s3:9000",region_name="us-east-1",aws_access_key_id=os.environ["STORAGE_ACCESS_KEY"],aws_secret_access_key=os.environ["STORAGE_SECRET_KEY"],config=Config(s3={"addressing_style":"path"},connect_timeout=2,read_timeout=3,retries={"max_attempts":0}))
bucket=os.environ["S3_STORAGE_FILES_BUCKET"]
s3.put_object(Bucket=bucket,Key="fixture",Body=b"fixture")
assert s3.get_object(Bucket=bucket,Key="fixture")["Body"].read()==b"fixture"
assert s3.list_objects_v2(Bucket=bucket)["KeyCount"]==1
upload=s3.create_multipart_upload(Bucket=bucket,Key="multipart")["UploadId"]
part=s3.upload_part(Bucket=bucket,Key="multipart",UploadId=upload,PartNumber=1,Body=b"part")
s3.complete_multipart_upload(Bucket=bucket,Key="multipart",UploadId=upload,MultipartUpload={"Parts":[{"ETag":part["ETag"],"PartNumber":1}]})
assert s3.get_object(Bucket=bucket,Key="multipart")["Body"].read()==b"part"
s3.delete_object(Bucket=bucket,Key="multipart")
for operation in [lambda:s3.create_bucket(Bucket="unowned-bucket"),lambda:s3.delete_bucket(Bucket=bucket),lambda:s3.list_objects_v2(Bucket="unowned-bucket")]:
 try:operation()
 except s3.exceptions.ClientError as error:assert error.response["ResponseMetadata"]["HTTPStatusCode"]==403
 else:raise AssertionError("storage operation was not denied")
for port in [8888,9333,9340,23646]:
 try:socket.create_connection(("s3",port),timeout=1)
 except OSError:pass
 else:raise AssertionError("internal API exposed")
try:urllib.request.urlopen("http://s3:9000/"+bucket+"/fixture")
except urllib.error.HTTPError as error:assert error.code==403
else:raise AssertionError("anonymous access succeeded")
print("Restricted object, multipart, listing and deletion checks pass; foreign buckets and internal APIs are inaccessible.")
'''
  result=run('docker','run','--rm','--name',client_container,'--network',network,'--env-file',str(envfile),'--entrypoint','python3','pp-audit-storage:seaweed','-c',script,timeout=40)
  print(result.stdout)
  run('docker','restart',container,timeout=30)
  for attempt in range(40):
   result=subprocess.run(['docker','exec',container,'python3','/opt/pp/storage.py','health'],capture_output=True,text=True)
   if not result.returncode:break
   if run("docker","inspect",container,"--format","{{.State.Running}}").stdout.strip() != "true":break
   time.sleep(1)
  else:raise RuntimeError('Storage restart failed')
  check='import sys,os;sys.path.insert(0,"/opt/pp");import storage;assert storage.client(os.environ).get_object(Bucket=os.environ["S3_STORAGE_FILES_BUCKET"],Key="fixture")["Body"].read()==b"fixture"'
  run('docker','exec',container,'python3','-c',check)
  print('Stored object survives restart.')
 finally:
  subprocess.run(['docker','rm','-fv',container,client_container],capture_output=True)
  subprocess.run(['docker','network','rm',network],capture_output=True)
