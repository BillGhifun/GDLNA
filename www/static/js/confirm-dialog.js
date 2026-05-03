/**
 * 自定义确认弹窗
 * @param {string} title - 弹窗标题
 * @param {string} message - 弹窗消息内容（支持Markdown）
 * @param {Function} onConfirm - 确认按钮回调函数
 * @param {string} footerText - 底部额外文本（可选，显示在滚动容器外）
 * @param {string} headerText - 顶部额外文本（可选，显示在滚动容器外）
 */
function showConfirmDialog(title, message, onConfirm, footerText, headerText) {
    // 创建遮罩层
    const overlay = document.createElement('div');
    overlay.className = 'confirm-dialog-overlay';

    // 创建弹窗卡片
    const card = document.createElement('div');
    card.className = 'confirm-dialog-card';

    // 标题
    const titleEl = document.createElement('h3');
    titleEl.className = 'confirm-dialog-title';
    titleEl.textContent = title;

    // 消息内容 - 支持Markdown解析
    const messageEl = document.createElement('div');
    messageEl.className = 'confirm-dialog-message';
    
    // 如果有顶部文本，先添加
    if (headerText) {
        const headerEl = document.createElement('p');
        headerEl.className = 'confirm-dialog-header';
        headerEl.textContent = headerText;
        messageEl.appendChild(headerEl);
    }
    
    // 如果marked库可用，则解析Markdown并添加滚动容器
    if (typeof marked !== 'undefined') {
        // 配置marked：支持GitHub风格的Markdown
        if (marked.setOptions) {
            marked.setOptions({
                breaks: true,  // 单个换行符解析为<br>
                gfm: true      // 启用GitHub风格Markdown
            });
        }
        
        // 只有当message有内容时才创建滚动容器（用于显示更新日志等长文本）
        if (message && message.trim()) {
            // 创建滚动容器
            const scrollContainer = document.createElement('div');
            scrollContainer.className = 'confirm-dialog-changelog';
            
            // 预处理：统一换行符格式
            const normalizedMessage = message.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
            
            // 解析并显示
            try {
                scrollContainer.innerHTML = marked.parse(normalizedMessage);
            } catch (e) {
                // 如果解析失败，显示原始文本
                scrollContainer.textContent = normalizedMessage;
            }
            
            messageEl.appendChild(scrollContainer);
        }
        
        // 如果有底部文本，添加在滚动容器外
        if (footerText) {
            const footerEl = document.createElement('p');
            footerEl.className = 'confirm-dialog-footer';
            footerEl.textContent = footerText;
            messageEl.appendChild(footerEl);
        }
    } else {
        // 降级方案：纯文本显示
        messageEl.textContent = message;
        if (footerText) {
            messageEl.textContent += '\n\n' + footerText;
        }
    }

    // 按钮容器
    const buttonContainer = document.createElement('div');
    buttonContainer.className = 'confirm-dialog-buttons';

    // 取消按钮
    const cancelBtn = document.createElement('button');
    cancelBtn.className = 'confirm-dialog-btn confirm-dialog-btn-cancel';
    cancelBtn.textContent = '取消';

    // 确认按钮
    const confirmBtn = document.createElement('button');
    confirmBtn.className = 'confirm-dialog-btn confirm-dialog-btn-confirm';
    confirmBtn.textContent = '确认';

    // 关闭弹窗函数
    const closeDialog = () => {
        overlay.style.animation = 'confirmDialogFadeIn 0.12s ease-in reverse forwards';
        card.style.animation = 'confirmDialogSlideIn 0.12s ease-in reverse forwards';
        setTimeout(() => overlay.remove(), 120);
    };

    // 按钮事件
    cancelBtn.onclick = closeDialog;
    confirmBtn.onclick = () => {
        closeDialog();
        if (onConfirm) onConfirm();
    };

    // 组装弹窗
    buttonContainer.appendChild(cancelBtn);
    buttonContainer.appendChild(confirmBtn);
    card.appendChild(titleEl);
    card.appendChild(messageEl);
    card.appendChild(buttonContainer);
    overlay.appendChild(card);
    document.body.appendChild(overlay);

    // 点击遮罩层关闭
    overlay.onclick = (e) => {
        if (e.target === overlay) closeDialog();
    };
}
