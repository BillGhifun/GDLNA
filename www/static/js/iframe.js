 document.addEventListener('click', function(e) {
   // 创建一个自定义事件，并传递一些数据，例如点击的坐标、目标等
   var event = new CustomEvent('iframeClick', {
     detail: {
       target: e.target.tagName,
       x: e.clientX,
       y: e.clientY,
       timestamp: Date.now()
     }
   });
   // 触发父窗口的事件
   //window.parent.dispatchEvent(event);
window.parent.document.body.click()

 });