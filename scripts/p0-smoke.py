import pathlib,tempfile,os,json,socket,subprocess,time,urllib.request,http.cookiejar,secrets,signal
import argparse
parser=argparse.ArgumentParser(description='Run isolated P0 HTTP smoke validation')
parser.add_argument('--binary',required=True)
parser.add_argument('--runner',required=True)
args=parser.parse_args()
repo=pathlib.Path(__file__).resolve().parents[1]
binary=str(pathlib.Path(args.binary).resolve());runner_binary=str(pathlib.Path(args.runner).resolve())
root=pathlib.Path(tempfile.mkdtemp(prefix='shizuku-p0-'))
(root/'config').mkdir();(root/'webui').symlink_to(repo/'webui',target_is_directory=True);(root/'litegraph.js-release').symlink_to(repo/'litegraph.js-release',target_is_directory=True)
with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
key=secrets.token_urlsafe(32);(root/'config/config.json').write_text(json.dumps({'webui':{'port':port,'password':key},'bots':[]}));os.chmod(root/'config/config.json',0o600)
log=open(root/'startup.log','w');os.chmod(root/'startup.log',0o600)
p=subprocess.Popen([binary],cwd=root,stdout=log,stderr=log)
base='http://127.0.0.1:'+str(port)
try:
 for attempt in range(100):
  if p.poll() is not None:raise RuntimeError('application exited during startup')
  try:
   if urllib.request.urlopen(base+'/login',timeout=1).status==200:break
  except OSError:time.sleep(.05)
 else:raise RuntimeError('readiness timed out')
 password=(root/'data/.admin-initial-password').read_text().strip()
 client=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
 req=urllib.request.Request(base+'/login',data=json.dumps({'username':'admin','password':password}).encode(),headers={'Content-Type':'application/json'})
 assert json.load(client.open(req))['code']==0
 for path in ['/','/api/config','/api/bots','/api/stats','/litegraph/build/litegraph.js']:
  resp=client.open(base+path);body=resp.read();assert resp.status==200 and body;assert key.encode() not in body and password.encode() not in body
 runner=subprocess.run([runner_binary,'-url',base,'-p',password,'-c','10','-n','100'],cwd=root,capture_output=True,text=True)
 assert runner.returncode==0,runner.stdout+runner.stderr
 print('PASS actual startup, login, APIs, bundled static assets, secret redaction')
 print('PASS repository HTTP concurrency runner: 100 successes, 0 failures, plugin cleaned up')
 p.send_signal(signal.SIGTERM);assert p.wait(timeout=10)==0
 log.flush();text=(root/'startup.log').read_text();assert password not in text and key not in text
 print('PASS graceful shutdown and no credentials in startup/shutdown logs')
 print('Smoke artifacts:',root)
finally:
 if p.poll() is None:p.terminate();p.wait(timeout=10)
 log.close()
