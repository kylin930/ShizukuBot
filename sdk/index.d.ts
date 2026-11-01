export type EventType = 'group_at_message'|'group_message'|'c2c_message'|'at_message'|'guild_message'|'direct_message'|'interaction'|'group_manage'|'c2c_manage'|'c2c_friend'|'subscribe_msg_status'|(string & {});
export interface Reply { kind:'text'|'image'|'rich_media'|'markdown'|'keyboard'|'video';text?:string;url?:string;keyboard?:string; }
export interface State {
  get<T = unknown>(key:string):Promise<T|null>;
  set(key:string,value:unknown):Promise<void>;
}
export interface Context<C = Record<string,unknown>> {
  readonly botId:string;readonly eventType:EventType;readonly messageId:string;
  readonly source:'group'|'c2c'|'guild'|'direct'|'';readonly userId:string;
  readonly groupId:string;readonly channelId:string;readonly guildId:string;readonly message:string;
  readonly args:readonly string[];readonly config:Readonly<C>;readonly state:State;
  reply(value:string|Reply):Promise<void>;
  replyImage(url:string):Promise<void>;
  replyRichMedia(text:string,url:string):Promise<void>;
  replyMarkdown(text:string):Promise<void>;
  replyKeyboard(markdown:string,keyboardJSON:string):Promise<void>;
  replyVideo(url:string):Promise<void>;
  log(message:string):Promise<void>;
}
export type Handler<C> = (ctx:Context<C>)=>void|Promise<void>;
export interface Plugin<C = Record<string,unknown>> {
  id:string;name:string;version:string;
  commands?:Record<string,Handler<C>>;
  events?:Partial<Record<EventType|'*',Handler<C>>>;
  load?(environment:{readonly botId:string;readonly config:Readonly<C>}):void|Promise<void>;
  start?():void|Promise<void>;stop?():void|Promise<void>;unload?():void|Promise<void>;
}
export function definePlugin<C = Record<string,unknown>>(plugin:Plugin<C>):Plugin<C>;
