// Global settings reuse the configuration save and draft guard.
(() => {
    'use strict';
    const U = window.AdminUI;
    const colors = [{value:'',label:'默认配色'}, {value:'#6750A4',label:'紫色'}, {value:'#0061A4',label:'蓝色'},
        {value:'#006A60',label:'青绿色'}, {value:'#984061',label:'粉色'}, {value:'custom',label:'自定义主色'}];
    function mount(host, data, scope) {
        let saved = U.clone(data.webui.appearance), uploaded = new Set(), validImage = true;
        const feedback = U.el('div', {id:'appearance-feedback', 'aria-live':'polite'});
        const kind = U.select('背景图片', [{value:'default',label:'默认背景'},{value:'url',label:'图片 URL'},{value:'upload',label:'本地图片'}], '', 'background-type');
        const url = U.el('mdui-text-field', {id:'background-url',label:'图片地址', placeholder:'https://…'});
        const file = U.el('input', {id:'background-file',type:'file',accept:'image/png,image/jpeg,image/webp', 'aria-label':'选择背景图片'});
        const upload = U.button('上传图片', uploadFile, 'background-upload');
        const fileRow = U.el('div', {class:'plugin-actions'}, [file, upload]);
        const mode = U.select('明暗模式', [{value:'auto',label:'跟随系统'},{value:'light',label:'浅色'},{value:'dark',label:'深色'}], '', 'theme-mode');
        const preset = U.select('主题配色', colors, '', 'theme-preset');
        const primary = U.el('mdui-text-field', {id:'theme-primary',label:'自定义主色',placeholder:'#6750A4',pattern:'#[0-9a-fA-F]{6}'});
        const preview = U.el('img', {class:'appearance-image-preview', id:'background-preview',alt:'背景预览',referrerPolicy:'no-referrer'});
        const sample = U.el('mdui-card', {variant:'outlined', class:'appearance-sample'}, [U.text('h3','配色预览'),U.text('p','文字与 Surface'),
            U.el('div',{class:'plugin-actions'},[U.el('mdui-button',{variant:'filled',text:'主要按钮'}),U.el('mdui-button',{variant:'outlined',text:'次要按钮'})])]);
        const appearance = U.el('mdui-card', {variant:'outlined',class:'config-card'}, [U.text('h2','外观'),
            U.el('div',{class:'preferences-grid'},[kind,url,fileRow,mode,preset,primary]),
            U.text('p','PNG、JPEG、WebP，最多 8 MiB、4096×4096 像素','plugin-muted'),feedback,
            U.el('div',{class:'appearance-preview'},[preview,sample]),
            U.el('div',{class:'plugin-actions'},[U.button('恢复默认背景',()=>{kind.value='default';url.value='';update();},'background-default'),
                U.button('取消预览',()=>reset(saved),'appearance-cancel')])]);
        const retention = U.select('数据保留时间', [7,30,90,180,365,0].map(n=>({value:String(n),label:n ? n+' 天':'永久保留'})), String(data.statistics.retention_days), 'stats-retention');
        const cleanup = U.el('mdui-text-field',{id:'stats-cleanup-time',label:'每天清理时间（HH:mm）',value:data.statistics.cleanup_time,pattern:'[0-2][0-9]:[0-5][0-9]'});
        const timezone = U.el('mdui-text-field',{id:'stats-timezone',label:'统计时区',value:data.statistics.timezone,readonly:true});
        const status = U.el('div',{id:'stats-storage-status','aria-live':'polite',class:'plugin-muted'});
        const statsFeedback = U.el('div',{id:'stats-settings-feedback'});
        const clean = U.button('立即清理过期数据', async()=>{
            if(scope.busy)return;scope.busy=true;U.controlsBusy(scope.container,true);
            try {const result=await scope.request('/api/stats/cleanup',{method:'POST',body:{}});if(scope.current()){showStatus(result);U.notice(statsFeedback,'过期数据已清理','success');}}
            catch(error){if(scope.current())U.notice(statsFeedback,U.errorMessage(error),'error');}
            finally {scope.busy=false;if(scope.current())U.controlsBusy(scope.container,false);}
        }, 'stats-cleanup');
        const statistics = U.el('mdui-card',{variant:'outlined',class:'config-card'},[U.text('h2','统计数据'),
            U.text('p','用户、群和收发计数只包含保留期内的数据；总体按各机器人分别去重后求和','plugin-muted'),
            U.el('div',{class:'preferences-grid'},[retention,cleanup,timezone]),U.text('p','立即清理使用已保存的设置，不会清空全部统计','plugin-muted'),status,statsFeedback,clean]);
        host.replaceChildren(appearance,statistics);
        function showStatus(value){status.textContent=`数据库：${value.state==='ready'?'正常':value.state==='degraded'?'异常':value.state} · ${((value.file_bytes||0)/1024).toFixed(1)} KiB · 上次清理：${value.last_cleanup||'暂无记录'}`;
            if(value.error||value.dropped_events)U.notice(statsFeedback,(value.error||'统计写入异常')+` · 丢弃记录 ${value.dropped_events||0}`,'error');}
        scope.request('/api/stats/storage').then(value=>{if(scope.current())showStatus(value);}).catch(error=>{if(scope.current())U.notice(statsFeedback,U.errorMessage(error),'error');});
        function readAppearance(){return {background:{type:kind.value,value:kind.value==='default'?'':url.value.trim()},theme:{mode:mode.value,primary_color:preset.value==='custom'?primary.value.trim():preset.value}};}
        function update(){
            url.hidden=kind.value==='default';url.readonly=kind.value==='upload';fileRow.hidden=kind.value!=='upload';primary.hidden=preset.value!=='custom';
            const value=readAppearance();primary.error=Boolean(value.theme.primary_color&&!/^#[0-9a-fA-F]{6}$/.test(value.theme.primary_color));
            if(primary.error){U.notice(feedback,'主色格式应为 #RRGGBB','error');return;}
            const publicValue=AppearanceUI.fromConfig(value);validImage=false;preview.src=publicValue.background_url||AppearanceUI.fallback;
            U.notice(feedback,'');AppearanceUI.apply(publicValue);
        }
        preview.addEventListener('load',()=>{validImage=true;U.notice(feedback,'');});
        preview.addEventListener('error',()=>{validImage=false;U.notice(feedback,'背景加载失败，配置未更改，可恢复默认背景','error');});
        async function removeUnused(){for(const item of uploaded){if(item===AppearanceUI.saved().background_url)continue;uploaded.delete(item);try{await U.request('/api/appearance/background',{method:'DELETE',body:{url:item}});}catch(error){console.warn('Unused background cleanup failed',error.message);}}}
        function reset(value){kind.value=value.background.type;url.value=value.background.value;mode.value=value.theme.mode;
            preset.value=colors.some(c=>c.value===value.theme.primary_color)?value.theme.primary_color:'custom';primary.value=value.theme.primary_color;update();void removeUnused();}
        async function uploadFile(){
            if(scope.busy)return;const selected=file.files[0];if(!selected){U.notice(feedback,'选择一张图片','error');return;}
            if(selected.size>8*1024*1024){U.notice(feedback,'图片不能超过 8 MiB','error');return;}
            scope.busy=true;U.controlsBusy(scope.container,true);
            try{const response=await fetch('/api/appearance/background',{method:'POST',headers:{'Content-Type':selected.type||'application/octet-stream'},body:selected,signal:scope.controller.signal});
                const value=await response.json();if(!response.ok||value.code!==0)throw Error(value.message||'图片上传失败');
                if(!scope.current())return;uploaded.add(value.data.url);kind.value='upload';url.value=value.data.url;update();U.notice(feedback,'图片已上传，保存配置后生效','success');}
            catch(error){if(scope.current())U.notice(feedback,U.errorMessage(error),'error');}
            finally{scope.busy=false;if(scope.current())U.controlsBusy(scope.container,false);}
        }
        for(const node of [kind,mode,preset])scope.on(node,'change',update);
        for(const node of [url,primary])scope.on(node,'input',update);
        AppearanceUI.commit(saved);
        reset(saved);
        return {read(){const a=readAppearance();if(primary.error)throw Error('主色格式应为 #RRGGBB');
                if(!/^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(cleanup.value)){cleanup.error=true;throw Error('清理时间应为 HH:mm');}cleanup.error=false;
                if(a.background.type!=='default'&&!validImage)throw Error('背景尚未加载，确认图片地址或恢复默认');
                return {appearance:a,statistics:{retention_days:Number(retention.value),cleanup_time:cleanup.value,timezone:timezone.value}};},
            commit(value){saved=U.clone(value);AppearanceUI.commit(saved);void removeUnused();},
            dispose(){AppearanceUI.restore();void removeUnused();}};
    }
    window.Preferences={mount};
})();
