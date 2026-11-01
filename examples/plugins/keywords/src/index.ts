import {definePlugin, type Context} from '@shizukubot/sdk';
type Config={keyword:string;reply:string};
async function respond(ctx:Context<Config>){if(ctx.message.includes(ctx.config.keyword))await ctx.reply(ctx.config.reply);}
export default definePlugin<Config>({id:'keywords',name:'Keyword replies',version:'1.0.0',events:{group_at_message:respond,group_message:respond,c2c_message:respond}});
