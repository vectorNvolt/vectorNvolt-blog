(()=>{var K,f,ve,it,D,he,ye,xe,ee,W,M,ke,oe,te,ne,we,z={},V=[],dt=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,G=Array.isArray;function C(t,e){for(var n in e)t[n]=e[n];return t}function ae(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function lt(t,e,n){var o,a,r,d={};for(r in e)r=="key"?o=e[r]:r=="ref"?a=e[r]:d[r]=e[r];if(arguments.length>2&&(d.children=arguments.length>3?K.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(r in t.defaultProps)d[r]===void 0&&(d[r]=t.defaultProps[r]);return B(t,d,o,a,null)}function B(t,e,n,o,a){var r={type:t,props:e,key:n,ref:o,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:a==null?++ve:a,__i:-1,__u:0};return a==null&&f.vnode!=null&&f.vnode(r),r}function I(t){return t.children}function j(t,e){this.props=t,this.context=e}function A(t,e){if(e==null)return t.__?A(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?A(t):null}function st(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,o=[],a=[],r=C({},e);r.__v=e.__v+1,f.vnode&&f.vnode(r),ie(t.__P,r,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,o,n==null?A(e):n,!!(32&e.__u),a),r.__v=e.__v,r.__.__k[r.__i]=r,Ce(o,r,a),e.__e=e.__=null,r.__e!=n&&Te(r)}}function Te(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),Te(t)}function re(t){(!t.__d&&(t.__d=!0)&&D.push(t)&&!q.__r++||he!=f.debounceRendering)&&((he=f.debounceRendering)||ye)(q)}function q(){try{for(var t,e=1;D.length;)D.length>e&&D.sort(xe),t=D.shift(),e=D.length,st(t)}finally{D.length=q.__r=0}}function Se(t,e,n,o,a,r,d,l,c,s,u){var h,i,g,m,T,x,y=o&&o.__k||V,p=e.length;for(c=_t(n,e,y,c,p),h=0;h<p;h++)(g=n.__k[h])!=null&&(i=g.__i!=-1&&y[g.__i]||z,g.__i=h,x=ie(t,g,i,a,r,d,l,c,s,u),m=g.__e,g.ref&&i.ref!=g.ref&&(i.ref&&de(i.ref,null,g),u.push(g.ref,g.__c||m,g)),T==null&&m!=null&&(T=m),4&g.__u?(c=Ee(g,c,t),i.__e&&(i.__e=null)):typeof g.type=="function"&&x!==void 0?c=x:m&&(c=m.nextSibling),g.__u&=-7);return n.__e=T,c}function _t(t,e,n,o,a){var r,d,l,c,s,u=n.length,h=u,i=0;for(t.__k=new Array(a),r=0;r<a;r++)(d=e[r])!=null&&typeof d!="boolean"&&typeof d!="function"?(typeof d=="string"||typeof d=="number"||typeof d=="bigint"||d.constructor==String?d=t.__k[r]=B(null,d,null,null,null):G(d)?d=t.__k[r]=B(I,{children:d},null,null,null):d.constructor===void 0&&d.__b>0?d=t.__k[r]=B(d.type,d.props,d.key,d.ref?d.ref:null,d.__v):t.__k[r]=d,c=r+i,d.__=t,d.__b=t.__b+1,l=null,(s=d.__i=ct(d,n,c,h))!=-1&&(h--,(l=n[s])&&(l.__u|=2)),l==null||l.__v==null?(s==-1&&(a>u?i--:a<u&&i++),typeof d.type!="function"&&(d.__u|=4)):s!=c&&(s==c-1?i--:s==c+1?i++:(s>c?i--:i++,d.__u|=4))):t.__k[r]=null;if(h)for(r=0;r<u;r++)(l=n[r])!=null&&!(2&l.__u)&&(l.__e==o&&(o=A(l)),De(l,l));return o}function Ee(t,e,n){var o,a;if(typeof t.type=="function"){for(o=t.__k,a=0;o&&a<o.length;a++)o[a]&&(o[a].__=t,e=Ee(o[a],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=A(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function ct(t,e,n,o){var a,r,d,l=t.key,c=t.type,s=e[n],u=s!=null&&(2&s.__u)==0;if(s===null&&l==null||u&&l==s.key&&c==s.type)return n;if(o>(u?1:0)){for(a=n-1,r=n+1;a>=0||r<e.length;)if((s=e[d=a>=0?a--:r++])!=null&&!(2&s.__u)&&l==s.key&&c==s.type)return d}return-1}function be(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||dt.test(e)?n:n+"px"}function F(t,e,n,o,a){var r,d;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof o=="string"&&(t.style.cssText=o=""),o)for(e in o)n&&e in n||be(t.style,e,"");if(n)for(e in n)o&&n[e]==o[e]||be(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")r=e!=(e=e.replace(ke,"$1")),d=e.toLowerCase(),e=d in t||e=="onFocusOut"||e=="onFocusIn"?d.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+r]=n,n?o?n[M]=o[M]:(n[M]=oe,t.addEventListener(e,r?ne:te,r)):t.removeEventListener(e,r?ne:te,r);else{if(a=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function me(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[W]==null)e[W]=oe++;else if(e[W]<n[M])return;return n(f.event?f.event(e):e)}}}function ie(t,e,n,o,a,r,d,l,c,s){var u,h,i,g,m,T,x,y,p,E,O,L,N,pe,U,Z,P=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(c=!!(32&n.__u),r=[l=e.__e=n.__e]),(u=f.__b)&&u(e);e:if(typeof P=="function"){h=d.length;try{if(p=e.props,E=P.prototype&&P.prototype.render,O=(u=P.contextType)&&o[u.__c],L=u?O?O.props.value:u.__:o,n.__c?y=(i=e.__c=n.__c).__=i.__E:(E?e.__c=i=new P(p,L):(e.__c=i=new j(p,L),i.constructor=P,i.render=gt),O&&O.sub(i),i.state||(i.state={}),i.__n=o,g=i.__d=!0,i.__h=[],i._sb=[]),E&&i.__s==null&&(i.__s=i.state),E&&P.getDerivedStateFromProps!=null&&(i.__s==i.state&&(i.__s=C({},i.__s)),C(i.__s,P.getDerivedStateFromProps(p,i.__s))),m=i.props,T=i.state,i.__v=e,g)E&&P.getDerivedStateFromProps==null&&i.componentWillMount!=null&&i.componentWillMount(),E&&i.componentDidMount!=null&&i.__h.push(i.componentDidMount);else{if(E&&P.getDerivedStateFromProps==null&&p!==m&&i.componentWillReceiveProps!=null&&i.componentWillReceiveProps(p,L),e.__v==n.__v||!i.__e&&i.shouldComponentUpdate!=null&&i.shouldComponentUpdate(p,i.__s,L)===!1){e.__v!=n.__v&&(i.props=p,i.state=i.__s,i.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(H){H&&(H.__=e)}),V.push.apply(i.__h,i._sb),i._sb=[],i.__h.length&&d.push(i),l=A(n);break e}i.componentWillUpdate!=null&&i.componentWillUpdate(p,i.__s,L),E&&i.componentDidUpdate!=null&&i.__h.push(function(){i.componentDidUpdate(m,T,x)})}if(i.context=L,i.props=p,i.__P=t,i.__e=!1,N=f.__r,pe=0,E)i.state=i.__s,i.__d=!1,N&&N(e),u=i.render(i.props,i.state,i.context),V.push.apply(i.__h,i._sb),i._sb=[];else do i.__d=!1,N&&N(e),u=i.render(i.props,i.state,i.context),i.state=i.__s;while(i.__d&&++pe<25);i.state=i.__s,i.getChildContext!=null&&(o=C(C({},o),i.getChildContext())),E&&!g&&i.getSnapshotBeforeUpdate!=null&&(x=i.getSnapshotBeforeUpdate(m,T)),U=u!=null&&u.type===I&&u.key==null?Ie(u.props.children):u,l=Se(t,G(U)?U:[U],e,n,o,a,r,d,l,c,s),i.base=e.__e,e.__u&=-161,i.__h.length&&d.push(i),y&&(i.__E=i.__=null)}catch(H){if(d.length=h,e.__v=null,c||r!=null){if(H.then){for(e.__u|=c?160:128;l&&l.nodeType==8&&l.nextSibling;)l=l.nextSibling;r!=null&&(r[r.indexOf(l)]=null),e.__e=l}else if(r!=null)for(Z=r.length;Z--;)ae(r[Z])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),H.then||Pe(e),f.__e(H,e,n)}}else r==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):l=e.__e=ut(n.__e,e,n,o,a,r,d,c,s);return(u=f.diffed)&&u(e),128&e.__u?void 0:l}function Pe(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(Pe))}function Ce(t,e,n){for(var o=0;o<n.length;o++)de(n[o],n[++o],n[++o]);f.__c&&f.__c(e,t),t.some(function(a){try{t=a.__h,a.__h=[],t.some(function(r){r.call(a)})}catch(r){f.__e(r,a.__v)}})}function Ie(t){return typeof t!="object"||t==null||t.__b>0?t:G(t)?t.map(Ie):t.constructor!==void 0?null:C({},t)}function ut(t,e,n,o,a,r,d,l,c){var s,u,h,i,g,m,T,x=n.props||z,y=e.props,p=e.type;if(p=="svg"?a="http://www.w3.org/2000/svg":p=="math"?a="http://www.w3.org/1998/Math/MathML":a||(a="http://www.w3.org/1999/xhtml"),r!=null){for(s=0;s<r.length;s++)if((g=r[s])&&"setAttribute"in g==!!p&&(p?g.localName==p:g.nodeType==3)){t=g,r[s]=null;break}}if(t==null){if(p==null)return document.createTextNode(y);t=document.createElementNS(a,p,y.is&&y),l&&(f.__m&&f.__m(e,r),l=!1),r=null}if(p==null)x===y||l&&t.data==y||(t.data=y);else{if(r=p=="textarea"&&y.defaultValue!=null?null:r&&K.call(t.childNodes),!l&&r!=null)for(x={},s=0;s<t.attributes.length;s++)x[(g=t.attributes[s]).name]=g.value;for(s in x)g=x[s],s=="dangerouslySetInnerHTML"?h=g:s=="children"||s in y||s=="value"&&"defaultValue"in y||s=="checked"&&"defaultChecked"in y||F(t,s,null,g,a);for(s in y)g=y[s],s=="children"?i=g:s=="dangerouslySetInnerHTML"?u=g:s=="value"?m=g:s=="checked"?T=g:l&&typeof g!="function"||x[s]===g||F(t,s,g,x[s],a);if(u)l||h&&(u.__html==h.__html||u.__html==t.innerHTML)||(t.innerHTML=u.__html),e.__k=[];else if(h&&(t.innerHTML=""),Se(e.type=="template"?t.content:t,G(i)?i:[i],e,n,o,p=="foreignObject"?"http://www.w3.org/1999/xhtml":a,r,d,r?r[0]:n.__k&&A(n,0),l,c),r!=null)for(s=r.length;s--;)ae(r[s]);l&&p!="textarea"||(s="value",p=="progress"&&m==null?t.removeAttribute("value"):m!=null&&(m!==t[s]||p=="progress"&&!m||p=="option"&&m!=x[s])&&F(t,s,m,x[s],a),s="checked",T!=null&&T!=t[s]&&F(t,s,T,x[s],a))}return t}function de(t,e,n){try{if(typeof t=="function"){var o=typeof t.__u=="function";o&&t.__u(),o&&e==null||(t.__u=t(e))}else t.current=e}catch(a){f.__e(a,n)}}function De(t,e,n){var o,a;if(f.unmount&&f.unmount(t),(o=t.ref)&&(o.current&&o.current!=t.__e||de(o,null,e)),(o=t.__c)!=null){if(o.componentWillUnmount)try{o.componentWillUnmount()}catch(r){f.__e(r,e)}o.base=o.__P=o.__n=null}if(o=t.__k)for(a=0;a<o.length;a++)o[a]&&De(o[a],e,n||typeof t.type!="function");n||ae(t.__e),t.__c=t.__=t.__e=void 0}function gt(t,e,n){return this.constructor(t,n)}function Le(t,e,n){var o,a,r,d;e==document&&(e=document.documentElement),f.__&&f.__(t,e),a=(o=typeof n=="function")?null:n&&n.__k||e.__k,r=[],d=[],ie(e,t=(!o&&n||e).__k=lt(I,null,[t]),a||z,z,e.namespaceURI,!o&&n?[n]:a?null:e.firstChild?K.call(e.childNodes):null,r,!o&&n?n:a?a.__e:e.firstChild,o,d),Ce(r,t,d),t.props.children=null}function Ae(t){function e(n){var o,a;return this.getChildContext||(o=new Set,(a={})[e.__c]=this,this.getChildContext=function(){return a},this.componentWillUnmount=function(){o=null},this.shouldComponentUpdate=function(r){this.props.value!=r.value&&o.forEach(function(d){d.__e=!0,re(d)})},this.sub=function(r){o.add(r);var d=r.componentWillUnmount;r.componentWillUnmount=function(){o&&o.delete(r),d&&d.call(r)}}),n.children}return e.__c="__cC"+we++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,o){return n.children(o)}).contextType=e,e}K=V.slice,f={__e:function(t,e,n,o){for(var a,r,d;e=e.__;)if((a=e.__c)&&!a.__)try{if((r=a.constructor)&&r.getDerivedStateFromError!=null&&(a.setState(r.getDerivedStateFromError(t)),d=a.__d),a.componentDidCatch!=null&&(a.componentDidCatch(t,o||{}),d=a.__d),d)return a.__E=a}catch(l){t=l}throw t}},ve=0,it=function(t){return t!=null&&t.constructor===void 0},j.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=C({},this.state),typeof t=="function"&&(t=t(C({},n),this.props)),t&&C(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),re(this))},j.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),re(this))},j.prototype.render=I,D=[],ye=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,xe=function(t,e){return t.__v.__b-e.__v.__b},q.__r=0,ee=Math.random().toString(8),W="__d"+ee,M="__a"+ee,ke=/(PointerCapture)$|Capture$/i,oe=0,te=me(!1),ne=me(!0),we=0;var $,b,le,He,Q=0,We=[],v=f,$e=v.__b,Oe=v.__r,Ne=v.diffed,Me=v.__c,Re=v.unmount,Ue=v.__;function X(t,e){v.__h&&v.__h(b,t,Q||e),Q=0;var n=b.__H||(b.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function R(t){return Q=1,ft(qe,t)}function ft(t,e,n){var o=X($++,2);if(o.t=t,!o.__c&&(o.__=[n?n(e):qe(void 0,e),function(l){var c=o.__N?o.__N[0]:o.__[0],s=o.t(c,l);c!==s&&(o.__N=[s,o.__[1]],o.__c.setState({}))}],o.__c=b,!b.__f)){var a=function(l,c,s){if(!o.__c.__H)return!0;var u=!1,h=o.__c.props!==l;if(o.__c.__H.__.some(function(g){if(g.__N){u=!0;var m=g.__[0];g.__=g.__N,g.__N=void 0,m!==g.__[0]&&(h=!0)}}),r){var i=r.call(this,l,c,s);return u?i||h:i}return!u||h};b.__f=!0;var r=b.shouldComponentUpdate,d=b.componentWillUpdate;b.componentWillUpdate=function(l,c,s){if(this.__e){var u=r;r=void 0,a(l,c,s),r=u}d&&d.call(this,l,c,s)},b.shouldComponentUpdate=a}return o.__N||o.__}function Be(t,e){var n=X($++,4);!v.__s&&Ve(n.__H,e)&&(n.__=t,n.u=e,b.__h.push(n))}function je(t){return Q=5,pt(function(){return{current:t}},[])}function pt(t,e){var n=X($++,7);return Ve(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function ze(t){var e=b.context[t.__c],n=X($++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(b)),e.props.value):t.__}function ht(){for(var t;t=We.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(Y),e.__h.some(se),e.__h=[]}catch(n){e.__h=[],v.__e(n,t.__v)}}}v.__b=function(t){b=null,$e&&$e(t)},v.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),Ue&&Ue(t,e)},v.__r=function(t){Oe&&Oe(t),$=0;var e=(b=t.__c).__H;e&&(le===b?(e.__h=[],b.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(Y),e.__h.some(se),e.__h=[],$=0)),le=b},v.diffed=function(t){Ne&&Ne(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(We.push(e)!==1&&He===v.requestAnimationFrame||((He=v.requestAnimationFrame)||bt)(ht)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),le=b=null},v.__c=function(t,e){e.some(function(n){try{n.__h.some(Y),n.__h=n.__h.filter(function(o){return!o.__||se(o)})}catch(o){e.some(function(a){a.__h&&(a.__h=[])}),e=[],v.__e(o,n.__v)}}),Me&&Me(t,e)},v.unmount=function(t){Re&&Re(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(o){try{Y(o)}catch(a){e=a}}),n.__H=void 0,e&&v.__e(e,n.__v))};var Fe=typeof requestAnimationFrame=="function";function bt(t){var e,n=function(){clearTimeout(o),Fe&&cancelAnimationFrame(e),setTimeout(t)},o=setTimeout(n,35);Fe&&(e=requestAnimationFrame(n))}function Y(t){var e=b,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),b=e}function se(t){var e=b;t.__c=t.__(),b=e}function Ve(t,e){return!t||t.length!==e.length||e.some(function(n,o){return n!==t[o]})}function qe(t,e){return typeof e=="function"?e(t):e}var Ke="(prefers-color-scheme: dark)";function mt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function vt(t){let e=t;for(;e;){let n=mt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(Ke).matches?"dark":"light"}function Ge(t){let[e,n]=R("light");return Be(()=>{let o=t.current;if(!o)return;let a=()=>n(vt(o.parentElement||o));a();let r=new MutationObserver(a),d={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};r.observe(document.documentElement,d),document.body&&r.observe(document.body,d);let l=window.matchMedia?window.matchMedia(Ke):null;return l&&l.addEventListener("change",a),()=>{r.disconnect(),l&&l.removeEventListener("change",a)}},[t]),e}var S={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",badgeRadius:13,badgeRadiusActive:15,badgeOnEdgeMin:60},yt=`
  --dg-bg: #f6f8fa;
  --dg-border: #d0d7de;
  --dg-box: #ffffff;
  --dg-box-stroke: #8c959f;
  --dg-outline: #8c959f;
  --dg-text: #1f2328;
  --dg-text-sub: #57606a;
  --dg-on-solid: #e8edf2;
  --dg-on-solid-sub: #d5dbe1;
  --dg-edge: #6e7781;
  --dg-accent: #ffd43b;
  --dg-accent-stroke: #b8860b;
  --dg-on-accent: #1b2631;
  --dg-badge: #e6ebf1;
  --dg-badge-stroke: #6e7781;
  --dg-badge-text: #1f2328;
  --dg-panel: #ffffff;
  --dg-panel-title: #8a6500;
  --dg-panel-text: #1f2328;
  --dg-hint: #6e7781;

  --dg-danger: #c0392b;
  --dg-danger-deep: #7b241c;
  --dg-danger-tint: rgba(192, 57, 43, 0.08);
  --dg-safe: #27ae60;
  --dg-safe-deep: #145a32;
  --dg-safe-tint: rgba(39, 174, 96, 0.08);
  --dg-info: #2e86c1;
  --dg-info-deep: #1b4f72;
  --dg-info-tint: rgba(46, 134, 193, 0.08);
  --dg-solid-dark: #34495e;
  --dg-solid-dark-stroke: #1b2631;

  --dg-chip-info: #0b4f9c;
  --dg-chip-info-border: #7fb3ff;
  --dg-chip-info-bg: rgba(127, 179, 255, 0.15);
  --dg-chip-danger: #9c2a20;
  --dg-chip-danger-border: #ff8a80;
  --dg-chip-danger-bg: rgba(255, 138, 128, 0.15);
  --dg-chip-safe: #1e6b3a;
  --dg-chip-safe-border: #7ddba0;
  --dg-chip-safe-bg: rgba(125, 219, 160, 0.15);
  --dg-chip-neutral: #57606a;
  --dg-chip-neutral-border: #b6bec8;
  --dg-chip-neutral-bg: rgba(182, 190, 200, 0.15);
`,Ye=`
  --dg-bg: #10151c;
  --dg-border: #2c3644;
  --dg-box: #1e2530;
  --dg-box-stroke: #4a5568;
  --dg-outline: #5d6d7e;
  --dg-text: #e8edf2;
  --dg-text-sub: #a9b4c0;
  --dg-on-solid: #e8edf2;
  --dg-on-solid-sub: #d5dbe1;
  --dg-edge: #7f8c9a;
  --dg-accent: #f4d03f;
  --dg-accent-stroke: #b7950b;
  --dg-on-accent: #1b2631;
  --dg-badge: #2c3644;
  --dg-badge-stroke: #7f8c9a;
  --dg-badge-text: #e8edf2;
  --dg-panel: #161d27;
  --dg-panel-title: #f4d03f;
  --dg-panel-text: #cfd8e3;
  --dg-hint: #8b97a5;

  --dg-danger: #c0392b;
  --dg-danger-deep: #7b241c;
  --dg-danger-tint: rgba(192, 57, 43, 0.10);
  --dg-safe: #27ae60;
  --dg-safe-deep: #145a32;
  --dg-safe-tint: rgba(39, 174, 96, 0.10);
  --dg-info: #2e86c1;
  --dg-info-deep: #1b4f72;
  --dg-info-tint: rgba(46, 134, 193, 0.10);
  --dg-solid-dark: #1b2631;
  --dg-solid-dark-stroke: #5d6d7e;

  --dg-chip-info: #7fb3ff;
  --dg-chip-info-border: #2f5d8a;
  --dg-chip-info-bg: rgba(127, 179, 255, 0.08);
  --dg-chip-danger: #ff8a80;
  --dg-chip-danger-border: #8a3a33;
  --dg-chip-danger-bg: rgba(255, 138, 128, 0.08);
  --dg-chip-safe: #7ddba0;
  --dg-chip-safe-border: #2f6b45;
  --dg-chip-safe-bg: rgba(125, 219, 160, 0.08);
  --dg-chip-neutral: #a9b4c0;
  --dg-chip-neutral-border: #4a5568;
  --dg-chip-neutral-bg: rgba(169, 180, 192, 0.08);
`,xt=`
  .dg-diagram { ${yt} }
  @media (prefers-color-scheme: dark) {
    .dg-diagram:not([data-dg-theme="light"]) { ${Ye} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${Ye} }

  /* frame */
  .dg-diagram {
    background: var(--dg-bg);
    border: 1px solid var(--dg-border);
    border-radius: 12px;
    padding: 12px;
    margin: 1.5em auto;
    max-width: 816px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  }
  .dg-diagram svg { width: 100%; height: auto; display: block; }

  /* typography */
  .dg-label { fill: var(--dg-text); font-size: 13px; }
  .dg-label-bold { font-weight: 600; }
  .dg-label-sm { font-size: 11px; }
  .dg-label-sub { fill: var(--dg-text-sub); font-size: 11px; }
  .dg-label-outline { fill: var(--dg-text-sub); font-size: 11px; letter-spacing: 0.02em; text-transform: uppercase; }

  /* outlines: dashed regions grouping boxes */
  .dg-outline rect { fill: none; stroke: var(--dg-outline); }
  .dg-outline-danger rect { stroke: var(--dg-danger); }
  .dg-outline-safe rect { stroke: var(--dg-safe); }
  .dg-outline-info rect { stroke: var(--dg-info); }
  .dg-outline-filled.dg-outline-neutral rect { fill: var(--dg-box); }
  .dg-outline-filled.dg-outline-danger rect { fill: var(--dg-danger-tint); }
  .dg-outline-filled.dg-outline-safe rect { fill: var(--dg-safe-tint); }
  .dg-outline-filled.dg-outline-info rect { fill: var(--dg-info-tint); }

  /* boxes: components */
  .dg-box rect { fill: var(--dg-box); stroke: var(--dg-box-stroke); }
  .dg-box-solid-danger rect { fill: var(--dg-danger); stroke: var(--dg-danger-deep); }
  .dg-box-solid-safe rect { fill: var(--dg-safe-deep); stroke: var(--dg-safe); }
  .dg-box-solid-info rect { fill: var(--dg-info-deep); stroke: var(--dg-info); }
  .dg-box-solid-dark rect { fill: var(--dg-solid-dark); stroke: var(--dg-solid-dark-stroke); }
  .dg-box-solid .dg-label { fill: var(--dg-on-solid); }
  .dg-box-solid .dg-label-sub { fill: var(--dg-on-solid-sub); }

  /* edges */
  .dg-edge { stroke: var(--dg-edge); }
  .dg-edge-active { stroke: var(--dg-accent-stroke); }
  .dg-marker { fill: var(--dg-edge); }
  .dg-marker-active { fill: var(--dg-accent-stroke); }

  /* badges: the interactive hotspots */
  .dg-badge { cursor: pointer; }
  .dg-badge circle { fill: var(--dg-badge); stroke: var(--dg-badge-stroke); }
  .dg-badge-active circle { fill: var(--dg-accent); stroke: var(--dg-accent-stroke); }
  .dg-badge:focus { outline: none; }
  .dg-badge:focus circle { stroke: var(--dg-accent-stroke); stroke-width: 2.5; }
  .dg-badge-label { fill: var(--dg-badge-text); font-size: 11px; font-weight: 700; pointer-events: none; }
  .dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }

  /* detail panel under the drawing */
  .dg-diagram .dg-panel {
    margin-top: 10px;
    padding: 12px 14px;
    background: var(--dg-panel);
    border: 1px solid var(--dg-border);
    border-radius: 8px;
    min-height: 64px;
  }
  .dg-diagram .dg-panel-title { color: var(--dg-panel-title); font-weight: 700; font-size: 14px; margin-bottom: 4px; }
  .dg-diagram .dg-panel-desc { color: var(--dg-panel-text); font-size: 13.5px; line-height: 1.5; margin: 0 0 8px; }
  .dg-diagram .dg-hint { color: var(--dg-hint); font-size: 13px; font-style: italic; margin: 0; }
  .dg-diagram .dg-chips { display: flex; flex-wrap: wrap; gap: 8px; }
  .dg-diagram .dg-chip { display: flex; align-items: baseline; gap: 5px; font-size: 11px; padding: 3px 10px; border-radius: 999px; border: 1px solid; }
  .dg-chip-id { font-weight: 700; }
  .dg-chip-desc { font-weight: 500; opacity: 0.85; }
  .dg-chip-info { color: var(--dg-chip-info); border-color: var(--dg-chip-info-border); background: var(--dg-chip-info-bg); }
  .dg-chip-danger { color: var(--dg-chip-danger); border-color: var(--dg-chip-danger-border); background: var(--dg-chip-danger-bg); }
  .dg-chip-safe { color: var(--dg-chip-safe); border-color: var(--dg-chip-safe-border); background: var(--dg-chip-safe-bg); }
  .dg-chip-neutral { color: var(--dg-chip-neutral); border-color: var(--dg-chip-neutral-border); background: var(--dg-chip-neutral-bg); }
`,Qe="dg-diagram-styles";function Xe(){if(typeof document=="undefined"||document.getElementById(Qe))return;let t=document.createElement("style");t.id=Qe,t.textContent=xt,document.head.appendChild(t)}var kt=0,Mt=Array.isArray;function _(t,e,n,o,a,r){e||(e={});var d,l,c=e;if("ref"in c)for(l in c={},e)l=="ref"?d=e[l]:c[l]=e[l];var s={type:t,props:c,key:n,ref:d,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--kt,__i:-1,__u:0,__source:a,__self:r};if(typeof t=="function"&&(d=t.defaultProps))for(l in d)c[l]===void 0&&(c[l]=d[l]);return f.vnode&&f.vnode(s),s}var Ze=Ae({markerPrefix:"dg-arrow"});function Tt(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function St(t,e="Edge"){for(let n=1;n<t.length;n++){let[o,a]=t[n-1],[r,d]=t[n];if(o!==r&&a!==d)throw new Error(`${e}: diagonal segment (${o},${a})\u2192(${r},${d}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function J({x:t,y:e,w:n,h:o,label:a,variant:r="neutral",filled:d}){return _("g",{class:`dg-outline dg-outline-${r}${d?" dg-outline-filled":""}`,children:[_("rect",{x:t,y:e,width:n,height:o,rx:S.outlineRadius,"stroke-width":S.outlineStroke,"stroke-dasharray":S.outlineDash}),a?_("text",{x:t+14,y:e+22,class:"dg-label dg-label-outline",children:a}):null]})}function k({x:t,y:e,w:n,h:o,label:a,sub:r,variant:d="default",small:l}){let c=d.startsWith("solid-"),s=r?e+o/2-6:e+o/2+5;return _("g",{class:`dg-box dg-box-${d}${c?" dg-box-solid":""}`,children:[_("rect",{x:t,y:e,width:n,height:o,rx:S.boxRadius,"stroke-width":S.boxStroke}),_("text",{x:t+n/2,y:s,"text-anchor":"middle",class:l?"dg-label dg-label-sm":"dg-label dg-label-bold",children:a}),r?_("text",{x:t+n/2,y:s+(l?14:18),"text-anchor":"middle",class:"dg-label dg-label-sub",children:r}):null]})}function _e({points:t,dashed:e,active:n,dir:o,id:a}){St(t,a?`Edge ${a}`:"Edge");let{markerPrefix:r}=ze(Ze),d=`${r}-${n?"active":"default"}`,l=o&&o!=="none";return _("polyline",{class:`dg-edge${n?" dg-edge-active":""}`,points:Tt(t),fill:"none","stroke-width":n?S.edgeStrokeActive:S.edgeStroke,"stroke-dasharray":e?S.edgeDash:void 0,"marker-end":l?`url(#${d})`:void 0,"marker-start":o==="both"?`url(#${d})`:void 0})}function et({id:t,label:e,name:n,x:o,y:a,active:r,pressed:d,onSelect:l,onHover:c}){let s=(u,h)=>{u.pointerType==="mouse"&&c&&c(h)};return _("g",{class:`dg-badge${r?" dg-badge-active":""}`,role:"button",tabindex:"0","aria-pressed":d,"aria-label":n?`${t}: ${n}`:t,onClick:()=>l(t),onPointerEnter:u=>s(u,t),onPointerLeave:u=>s(u,null),onKeyDown:u=>{(u.key==="Enter"||u.key===" ")&&(u.preventDefault(),l(t))},children:[_("circle",{cx:o,cy:a,r:r?S.badgeRadiusActive:S.badgeRadius,"stroke-width":"1.5"}),_("text",{x:o,y:a+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function tt(){let[t,e]=R(null),[n,o]=R(null);return{shownId:n||t,pinnedId:t,select:r=>e(d=>d===r?null:r),setHover:o}}function Et({tone:t="neutral",id:e,text:n}){return _("span",{class:`dg-chip dg-chip-${t}`,children:[_("span",{class:"dg-chip-id",children:e}),n?_("span",{class:"dg-chip-desc",children:n}):null]})}function nt({id:t,title:e,desc:n,chips:o,hint:a,children:r}){return _("div",{class:"dg-panel","aria-live":"polite",children:e?_(I,{children:[_("div",{class:"dg-panel-title",children:[t?_(I,{children:[t," \u2014 "]}):null,e]}),n?_("div",{class:"dg-panel-desc",children:n}):null,o&&o.length?_("div",{class:"dg-chips",children:o.map(d=>_(Et,{...d},`${d.tone}-${d.id}`))}):null,r]}):_("div",{class:"dg-hint",children:a})})}function Je({id:t,active:e}){return _("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:_("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}function rt({id:t,viewH:e,viewW:n=960,label:o,footer:a,children:r}){Xe();let d=je(null),l=Ge(d),c=`dg-arrow-${t}`;return _("div",{class:"dg-diagram","data-dg-theme":l,ref:d,children:[_(Ze.Provider,{value:{markerPrefix:c},children:_("svg",{viewBox:`0 0 ${n} ${e}`,role:"group","aria-label":o,children:[_("defs",{children:[_(Je,{id:`${c}-default`}),_(Je,{id:`${c}-active`,active:!0})]}),r]})}),a]})}var ce={T1:"Data exfiltration",T2:"Supply-chain poisoning",T3:"Destructive actions",T4:"Lateral movement",T5:"Out-of-band persistence",T6:"Privilege escalation",T7:"Resource exhaustion",T8:"Control-plane compromise",T9:"Orchestrator hijack",L1:"Execution boundary",L2:"Resource governance",L3:"Filesystem hardening",L4:"Network policy",L5:"Credential brokering",L6:"Action governance",L7:"Control-plane scoping"},ue={"IP-1":{name:"Operator I/O",iface:"Terminal \u2014 stdin/stdout via the operator\u2019s terminal emulator",desc:"Operator \u2194 host orchestrator. The only human-in-the-loop gate, and the one edge where sandbox-originated bytes reach an interpreter with host-side effect: the terminal emulator.",t:["T1","T9"],l:["L6"]},"IP-2":{name:"Docker control plane",iface:"Container runtime API \u2014 the daemon socket",desc:"Host orchestrator \u2194 Docker Engine. The privileged side of the boundary: whatever can reach this API can create, exec into, or tear down any container.",t:["T8"],l:["L7"]},"IP-3":{name:"Exec channel",iface:"TTY-attached exec socket (docker exec stdin/stdout)",desc:"Host orchestrator \u2194 process inside the container. Trusted \u2194 untrusted data plane; if the harness decides what happened by reading this channel, the agent owns its own monitor.",t:["T7","T9"],l:["L6"]},"IP-4a":{name:"Kernel/Sentry boundary (in)",iface:"Linux syscall ABI \u2014 container \u2192 gVisor Sentry",desc:"The isolation boundary proper. Under runsc the container\u2019s syscalls are intercepted by the userspace Sentry; under bare runc this hop goes straight to the host kernel.",t:["T6","T7"],l:["L1","L2"]},"IP-4b":{name:"Kernel/Sentry boundary (out)",iface:"Linux syscall ABI \u2014 Sentry \u2192 host kernel",desc:"The Sentry emulates or forwards on the container\u2019s behalf. A kernel-level exploit here is contained to the Sentry process, not host root \u2014 T6g\u2013l is the residual set.",t:["T6"],l:["L1"]},"IP-5":{name:"Container filesystem",iface:"Mounted filesystem \u2014 rootfs, volumes, any bind mount",desc:"Container process \u2194 storage, and the host if anything is mounted or exported. Ephemeral \u2194 persistent: where installs land, where damage lands, and what survives a reset.",t:["T2","T3","T5"],l:["L3"]},"IP-6":{name:"Network egress",iface:"Container network namespace (bridge / veth)",desc:"Container \u2194 external networks and services. Sandbox \u2194 the outside world: the legitimate API call, the exfiltration POST, the registry fetch and the metadata query all ride the same edge.",t:["T1","T2","T4"],l:["L4","L5"]}};var Pt=720,w={operator:{x:20,y:20,w:120,h:56},terminal:{x:180,y:20,w:200,h:56},orchestrator:{x:50,y:140,w:220,h:80},engine:{x:340,y:140,w:180,h:80},agent:{x:370,y:290,w:290,h:70},credentials:{x:370,y:400,w:120,h:60},fs:{x:540,y:400,w:120,h:60},sentry:{x:340,y:500,w:350,h:56},kernel:{x:50,y:600,w:640,h:70},api:{x:770,y:140,w:150,h:56},attacker:{x:770,y:230,w:150,h:56},registry:{x:770,y:320,w:150,h:56},metadata:{x:770,y:410,w:150,h:56},internal:{x:770,y:500,w:150,h:56}},ge={host:{x:20,y:100,w:700,h:600},container:{x:340,y:250,w:350,h:230},outside:{x:750,y:100,w:190,h:600}},ot=[{id:"IP-1",points:[[280,76],[280,108],[160,108],[160,140]],badge:[220,108],dir:"both"},{id:"IP-2",points:[[270,180],[340,180]],badge:[305,180],dir:"end"},{id:"IP-3",points:[[160,220],[160,325],[370,325]],badge:[265,325],dir:"both"},{id:"IP-4a",points:[[515,360],[515,500]],badge:[515,430],dir:"end"},{id:"IP-4b",points:[[515,556],[515,600]],badge:[545,578],dir:"end"},{id:"IP-5",points:[[600,360],[600,400]],badge:[630,380],dir:"both"},{id:"IP-6",points:[[660,325],[740,325]],badge:[705,325],dir:"both"}],Ct=[{points:[[140,48],[180,48]],dir:"both"},{points:[[430,220],[430,250]],dir:"end"},{points:[[80,220],[80,600]],dashed:!0},{points:[[740,168],[740,528]]},{points:[[740,168],[770,168]],dir:"end"},{points:[[740,258],[770,258]],dir:"end"},{points:[[740,348],[770,348]],dir:"end"},{points:[[740,438],[770,438]],dir:"end"},{points:[[740,528],[770,528]],dir:"end"}];function fe({id:t="boundary"}){let{shownId:e,pinnedId:n,select:o,setHover:a}=tt(),r=e?ue[e]:null,d=r?[...r.t.map(l=>({tone:"danger",id:l,text:ce[l]})),...r.l.map(l=>({tone:"safe",id:l,text:ce[l]}))]:[];return _(rt,{id:t,viewH:Pt,label:"System boundary of a container-sandboxed agent harness with its six interaction points",footer:_(nt,{id:e,title:r==null?void 0:r.name,desc:r?`${r.iface}. ${r.desc}`:void 0,chips:d,hint:"Hover or click a numbered badge to see the interface behind that interaction point, the threat vectors that land on it and the layers that harden it."}),children:[_(J,{...ge.host,label:"HOST SYSTEM"}),_(J,{...ge.container,label:"AGENT CONTAINER",variant:"danger",filled:!0}),_(J,{...ge.outside,label:"OUTSIDE",variant:"danger"}),_(k,{...w.operator,label:"Operator",sub:"(human)"}),_(k,{...w.terminal,label:"Terminal emulator",sub:"interprets ESC/OSC \xB7 T9"}),_(k,{...w.orchestrator,label:"Host orchestrator",sub:"LangGraph harness \xB7 M and I"}),_(k,{...w.engine,label:"Docker Engine",sub:"daemon socket \xB7 T8"}),_(k,{...w.agent,label:"Agent process",sub:"untrusted by construction \xB7 U"}),_(k,{...w.credentials,label:"Credentials",sub:"the prize \xB7 T1",small:!0}),_(k,{...w.fs,label:"Filesystem",sub:"writable, reused \xB7 T5",small:!0}),_(k,{...w.sentry,label:"gVisor Sentry",sub:"userspace kernel \xB7 T6g\u2013l residual",variant:"solid-safe"}),_(k,{...w.kernel,label:"HOST KERNEL",sub:"shared by everything on the box \xB7 T6 T7",variant:"solid-dark"}),_(k,{...w.api,label:"Model API",sub:"legitimate egress",small:!0}),_(k,{...w.attacker,label:"Attacker host",sub:"exfil sink, injection \xB7 T1 T9",small:!0}),_(k,{...w.registry,label:"Package registry",sub:"slopsquatted deps \xB7 T2",small:!0}),_(k,{...w.metadata,label:"Cloud metadata",sub:"169.254.169.254 \xB7 T4",small:!0}),_(k,{...w.internal,label:"Internal network",sub:"host services \xB7 T4",small:!0}),Ct.map((l,c)=>_(_e,{...l},c)),ot.map(l=>_(_e,{id:l.id,points:l.points,dir:l.dir,active:e===l.id},l.id)),ot.map(l=>_(et,{id:l.id,label:l.id.replace("IP-",""),name:ue[l.id].name,x:l.badge[0],y:l.badge[1],active:e===l.id,pressed:n===l.id,onSelect:o,onHover:a},l.id))]})}var It={"island-boundary":{id:"boundary"}};function at(){for(let[t,e]of Object.entries(It)){let n=document.getElementById(t);n&&Le(_(fe,{...e}),n)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",at):at();})();
