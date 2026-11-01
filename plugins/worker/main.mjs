import readline from 'node:readline';
import {pathToFileURL} from 'node:url';
// stdout is reserved for protocol frames. Plugin console output is bounded by the host.
const write = process.stdout.write.bind(process.stdout);
console.log = console.info = console.warn = console.error = (...args) => process.stderr.write(args.map(String).join(' ').slice(0,4096)+'\n');
const pending = new Map(); let sequence = 0, plugin, environment;
const send = frame => write(JSON.stringify({v:1,...frame})+'\n');
function capability(parent, type, payload) {
  const id = `cap-${++sequence}`;
  return new Promise((resolve,reject) => {pending.set(id,{resolve,reject});send({id,parent,type,payload});});
}
function context(request, data) {
  const call = (type,payload) => capability(request.id,type,payload);
  const reply = value => call('reply',typeof value==='string'?{kind:'text',text:value}:value);
  return Object.freeze({...data.event, args:data.args??[],config:structuredClone(environment.config),
    reply, replyImage:url=>reply({kind:'image',url}),
    replyRichMedia:(text,url)=>reply({kind:'rich_media',text,url}),
    replyMarkdown:text=>reply({kind:'markdown',text}),
    replyKeyboard:(text,keyboard)=>reply({kind:'keyboard',text,keyboard}),
    replyVideo:url=>reply({kind:'video',url}),
    state:{get:key=>call('state.get',{key}),set:(key,value)=>call('state.set',{key,value})},
    log:message=>call('log',{message:String(message).slice(0,4096)})});
}
async function handle(request) {
  if (request.v!==1 || typeof request.id!=='string') throw new Error('invalid protocol frame');
  if (request.type==='load') {
    environment=request.payload;
    plugin=(await import(pathToFileURL(environment.entry).href)).default;
    if (!plugin || plugin.id!==environment.id || typeof plugin.version!=='string') throw new Error('plugin export does not match manifest');
    const commands=Object.keys(plugin.commands??{}), events=Object.keys(plugin.events??{});
    if ([...Object.values(plugin.commands??{}),...Object.values(plugin.events??{})].some(h=>typeof h!=='function')) throw new Error('handlers must be functions');
    await plugin.load?.(Object.freeze({botId:environment.botId,config:structuredClone(environment.config)}));
    await plugin.start?.();
    return {commands,events,version:plugin.version};
  }
  if (request.type==='event') {
    const data=request.payload, ctx=context(request,data), errors=[];
    const handlers=[];
    if(data.command && plugin.commands?.[data.command]) handlers.push(plugin.commands[data.command]);
    if(plugin.events?.[data.event.eventType]) handlers.push(plugin.events[data.event.eventType]);
    if(plugin.events?.['*']) handlers.push(plugin.events['*']);
    for(const handler of handlers) {try {await handler(ctx);} catch(error) {errors.push(String(error?.message??error).slice(0,4096));}}
    if(errors.length) throw new Error(errors.join('; '));
    return {handled:handlers.length};
  }
  if(request.type==='stop'){try {await plugin?.stop?.();} finally {await plugin?.unload?.();} return {};}
  throw new Error('unknown host request');
}
const lines=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
let busy=false;
lines.on('line',line=>{
  if(Buffer.byteLength(line)>1048576) process.exit(2);
  let request;try {request=JSON.parse(line);} catch {process.exit(2);}
  if(request.type==='capability_result') {
    const callback=pending.get(request.id);if(!callback)return;
    pending.delete(request.id);request.error?callback.reject(new Error(request.error)):callback.resolve(request.payload);return;
  }
  if(busy){send({id:request.id,type:'result',error:'worker is busy'});return;}
  busy=true;
  handle(request).then(payload=>{busy=false;send({id:request.id,type:'result',payload});},error=>{busy=false;send({id:request.id,type:'result',error:String(error?.message??error).slice(0,4096)});});
});
