import {definePlugin} from '@shizukubot/sdk';
export default definePlugin({id:'hello',name:'Hello / Ping',version:'1.0.0',commands:{ping:async(ctx)=>{await ctx.reply('Pong!');},echo:async(ctx)=>{await ctx.reply(ctx.args.join(' '));}}});
