(()=>{var K,p,he,at,I,ge,me,be,ee,W,O,ve,oe,te,ne,ye,z={},V=[],it=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,G=Array.isArray;function P(t,e){for(var n in e)t[n]=e[n];return t}function ae(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function dt(t,e,n){var o,a,r,d={};for(r in e)r=="key"?o=e[r]:r=="ref"?a=e[r]:d[r]=e[r];if(arguments.length>2&&(d.children=arguments.length>3?K.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(r in t.defaultProps)d[r]===void 0&&(d[r]=t.defaultProps[r]);return j(t,d,o,a,null)}function j(t,e,n,o,a){var r={type:t,props:e,key:n,ref:o,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:a==null?++he:a,__i:-1,__u:0};return a==null&&p.vnode!=null&&p.vnode(r),r}function C(t){return t.children}function B(t,e){this.props=t,this.context=e}function N(t,e){if(e==null)return t.__?N(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?N(t):null}function st(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,o=[],a=[],r=P({},e);r.__v=e.__v+1,p.vnode&&p.vnode(r),ie(t.__P,r,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,o,n==null?N(e):n,!!(32&e.__u),a),r.__v=e.__v,r.__.__k[r.__i]=r,Pe(o,r,a),e.__e=e.__=null,r.__e!=n&&xe(r)}}function xe(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),xe(t)}function re(t){(!t.__d&&(t.__d=!0)&&I.push(t)&&!q.__r++||ge!=p.debounceRendering)&&((ge=p.debounceRendering)||me)(q)}function q(){try{for(var t,e=1;I.length;)I.length>e&&I.sort(be),t=I.shift(),e=I.length,st(t)}finally{I.length=q.__r=0}}function ke(t,e,n,o,a,r,d,l,s,c,_){var h,i,u,b,k,x,y=o&&o.__k||V,f=e.length;for(s=lt(n,e,y,s,f),h=0;h<f;h++)(u=n.__k[h])!=null&&(i=u.__i!=-1&&y[u.__i]||z,u.__i=h,x=ie(t,u,i,a,r,d,l,s,c,_),b=u.__e,u.ref&&i.ref!=u.ref&&(i.ref&&de(i.ref,null,u),_.push(u.ref,u.__c||b,u)),k==null&&b!=null&&(k=b),4&u.__u?(s=Te(u,s,t),i.__e&&(i.__e=null)):typeof u.type=="function"&&x!==void 0?s=x:b&&(s=b.nextSibling),u.__u&=-7);return n.__e=k,s}function lt(t,e,n,o,a){var r,d,l,s,c,_=n.length,h=_,i=0;for(t.__k=new Array(a),r=0;r<a;r++)(d=e[r])!=null&&typeof d!="boolean"&&typeof d!="function"?(typeof d=="string"||typeof d=="number"||typeof d=="bigint"||d.constructor==String?d=t.__k[r]=j(null,d,null,null,null):G(d)?d=t.__k[r]=j(C,{children:d},null,null,null):d.constructor===void 0&&d.__b>0?d=t.__k[r]=j(d.type,d.props,d.key,d.ref?d.ref:null,d.__v):t.__k[r]=d,s=r+i,d.__=t,d.__b=t.__b+1,l=null,(c=d.__i=ct(d,n,s,h))!=-1&&(h--,(l=n[c])&&(l.__u|=2)),l==null||l.__v==null?(c==-1&&(a>_?i--:a<_&&i++),typeof d.type!="function"&&(d.__u|=4)):c!=s&&(c==s-1?i--:c==s+1?i++:(c>s?i--:i++,d.__u|=4))):t.__k[r]=null;if(h)for(r=0;r<_;r++)(l=n[r])!=null&&!(2&l.__u)&&(l.__e==o&&(o=N(l)),Ie(l,l));return o}function Te(t,e,n){var o,a;if(typeof t.type=="function"){for(o=t.__k,a=0;o&&a<o.length;a++)o[a]&&(o[a].__=t,e=Te(o[a],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=N(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function ct(t,e,n,o){var a,r,d,l=t.key,s=t.type,c=e[n],_=c!=null&&(2&c.__u)==0;if(c===null&&l==null||_&&l==c.key&&s==c.type)return n;if(o>(_?1:0)){for(a=n-1,r=n+1;a>=0||r<e.length;)if((c=e[d=a>=0?a--:r++])!=null&&!(2&c.__u)&&l==c.key&&s==c.type)return d}return-1}function pe(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||it.test(e)?n:n+"px"}function F(t,e,n,o,a){var r,d;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof o=="string"&&(t.style.cssText=o=""),o)for(e in o)n&&e in n||pe(t.style,e,"");if(n)for(e in n)o&&n[e]==o[e]||pe(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")r=e!=(e=e.replace(ve,"$1")),d=e.toLowerCase(),e=d in t||e=="onFocusOut"||e=="onFocusIn"?d.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+r]=n,n?o?n[O]=o[O]:(n[O]=oe,t.addEventListener(e,r?ne:te,r)):t.removeEventListener(e,r?ne:te,r);else{if(a=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function fe(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[W]==null)e[W]=oe++;else if(e[W]<n[O])return;return n(p.event?p.event(e):e)}}}function ie(t,e,n,o,a,r,d,l,s,c){var _,h,i,u,b,k,x,y,f,T,D,E,M,ue,L,Z,w=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(s=!!(32&n.__u),r=[l=e.__e=n.__e]),(_=p.__b)&&_(e);e:if(typeof w=="function"){h=d.length;try{if(f=e.props,T=w.prototype&&w.prototype.render,D=(_=w.contextType)&&o[_.__c],E=_?D?D.props.value:_.__:o,n.__c?y=(i=e.__c=n.__c).__=i.__E:(T?e.__c=i=new w(f,E):(e.__c=i=new B(f,E),i.constructor=w,i.render=ut),D&&D.sub(i),i.state||(i.state={}),i.__n=o,u=i.__d=!0,i.__h=[],i._sb=[]),T&&i.__s==null&&(i.__s=i.state),T&&w.getDerivedStateFromProps!=null&&(i.__s==i.state&&(i.__s=P({},i.__s)),P(i.__s,w.getDerivedStateFromProps(f,i.__s))),b=i.props,k=i.state,i.__v=e,u)T&&w.getDerivedStateFromProps==null&&i.componentWillMount!=null&&i.componentWillMount(),T&&i.componentDidMount!=null&&i.__h.push(i.componentDidMount);else{if(T&&w.getDerivedStateFromProps==null&&f!==b&&i.componentWillReceiveProps!=null&&i.componentWillReceiveProps(f,E),e.__v==n.__v||!i.__e&&i.shouldComponentUpdate!=null&&i.shouldComponentUpdate(f,i.__s,E)===!1){e.__v!=n.__v&&(i.props=f,i.state=i.__s,i.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(A){A&&(A.__=e)}),V.push.apply(i.__h,i._sb),i._sb=[],i.__h.length&&d.push(i),l=N(n);break e}i.componentWillUpdate!=null&&i.componentWillUpdate(f,i.__s,E),T&&i.componentDidUpdate!=null&&i.__h.push(function(){i.componentDidUpdate(b,k,x)})}if(i.context=E,i.props=f,i.__P=t,i.__e=!1,M=p.__r,ue=0,T)i.state=i.__s,i.__d=!1,M&&M(e),_=i.render(i.props,i.state,i.context),V.push.apply(i.__h,i._sb),i._sb=[];else do i.__d=!1,M&&M(e),_=i.render(i.props,i.state,i.context),i.state=i.__s;while(i.__d&&++ue<25);i.state=i.__s,i.getChildContext!=null&&(o=P(P({},o),i.getChildContext())),T&&!u&&i.getSnapshotBeforeUpdate!=null&&(x=i.getSnapshotBeforeUpdate(b,k)),L=_!=null&&_.type===C&&_.key==null?Ce(_.props.children):_,l=ke(t,G(L)?L:[L],e,n,o,a,r,d,l,s,c),i.base=e.__e,e.__u&=-161,i.__h.length&&d.push(i),y&&(i.__E=i.__=null)}catch(A){if(d.length=h,e.__v=null,s||r!=null){if(A.then){for(e.__u|=s?160:128;l&&l.nodeType==8&&l.nextSibling;)l=l.nextSibling;r!=null&&(r[r.indexOf(l)]=null),e.__e=l}else if(r!=null)for(Z=r.length;Z--;)ae(r[Z])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),A.then||we(e),p.__e(A,e,n)}}else r==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):l=e.__e=_t(n.__e,e,n,o,a,r,d,s,c);return(_=p.diffed)&&_(e),128&e.__u?void 0:l}function we(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(we))}function Pe(t,e,n){for(var o=0;o<n.length;o++)de(n[o],n[++o],n[++o]);p.__c&&p.__c(e,t),t.some(function(a){try{t=a.__h,a.__h=[],t.some(function(r){r.call(a)})}catch(r){p.__e(r,a.__v)}})}function Ce(t){return typeof t!="object"||t==null||t.__b>0?t:G(t)?t.map(Ce):t.constructor!==void 0?null:P({},t)}function _t(t,e,n,o,a,r,d,l,s){var c,_,h,i,u,b,k,x=n.props||z,y=e.props,f=e.type;if(f=="svg"?a="http://www.w3.org/2000/svg":f=="math"?a="http://www.w3.org/1998/Math/MathML":a||(a="http://www.w3.org/1999/xhtml"),r!=null){for(c=0;c<r.length;c++)if((u=r[c])&&"setAttribute"in u==!!f&&(f?u.localName==f:u.nodeType==3)){t=u,r[c]=null;break}}if(t==null){if(f==null)return document.createTextNode(y);t=document.createElementNS(a,f,y.is&&y),l&&(p.__m&&p.__m(e,r),l=!1),r=null}if(f==null)x===y||l&&t.data==y||(t.data=y);else{if(r=f=="textarea"&&y.defaultValue!=null?null:r&&K.call(t.childNodes),!l&&r!=null)for(x={},c=0;c<t.attributes.length;c++)x[(u=t.attributes[c]).name]=u.value;for(c in x)u=x[c],c=="dangerouslySetInnerHTML"?h=u:c=="children"||c in y||c=="value"&&"defaultValue"in y||c=="checked"&&"defaultChecked"in y||F(t,c,null,u,a);for(c in y)u=y[c],c=="children"?i=u:c=="dangerouslySetInnerHTML"?_=u:c=="value"?b=u:c=="checked"?k=u:l&&typeof u!="function"||x[c]===u||F(t,c,u,x[c],a);if(_)l||h&&(_.__html==h.__html||_.__html==t.innerHTML)||(t.innerHTML=_.__html),e.__k=[];else if(h&&(t.innerHTML=""),ke(e.type=="template"?t.content:t,G(i)?i:[i],e,n,o,f=="foreignObject"?"http://www.w3.org/1999/xhtml":a,r,d,r?r[0]:n.__k&&N(n,0),l,s),r!=null)for(c=r.length;c--;)ae(r[c]);l&&f!="textarea"||(c="value",f=="progress"&&b==null?t.removeAttribute("value"):b!=null&&(b!==t[c]||f=="progress"&&!b||f=="option"&&b!=x[c])&&F(t,c,b,x[c],a),c="checked",k!=null&&k!=t[c]&&F(t,c,k,x[c],a))}return t}function de(t,e,n){try{if(typeof t=="function"){var o=typeof t.__u=="function";o&&t.__u(),o&&e==null||(t.__u=t(e))}else t.current=e}catch(a){p.__e(a,n)}}function Ie(t,e,n){var o,a;if(p.unmount&&p.unmount(t),(o=t.ref)&&(o.current&&o.current!=t.__e||de(o,null,e)),(o=t.__c)!=null){if(o.componentWillUnmount)try{o.componentWillUnmount()}catch(r){p.__e(r,e)}o.base=o.__P=o.__n=null}if(o=t.__k)for(a=0;a<o.length;a++)o[a]&&Ie(o[a],e,n||typeof t.type!="function");n||ae(t.__e),t.__c=t.__=t.__e=void 0}function ut(t,e,n){return this.constructor(t,n)}function Se(t,e,n){var o,a,r,d;e==document&&(e=document.documentElement),p.__&&p.__(t,e),a=(o=typeof n=="function")?null:n&&n.__k||e.__k,r=[],d=[],ie(e,t=(!o&&n||e).__k=dt(C,null,[t]),a||z,z,e.namespaceURI,!o&&n?[n]:a?null:e.firstChild?K.call(e.childNodes):null,r,!o&&n?n:a?a.__e:e.firstChild,o,d),Pe(r,t,d),t.props.children=null}function Ee(t){function e(n){var o,a;return this.getChildContext||(o=new Set,(a={})[e.__c]=this,this.getChildContext=function(){return a},this.componentWillUnmount=function(){o=null},this.shouldComponentUpdate=function(r){this.props.value!=r.value&&o.forEach(function(d){d.__e=!0,re(d)})},this.sub=function(r){o.add(r);var d=r.componentWillUnmount;r.componentWillUnmount=function(){o&&o.delete(r),d&&d.call(r)}}),n.children}return e.__c="__cC"+ye++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,o){return n.children(o)}).contextType=e,e}K=V.slice,p={__e:function(t,e,n,o){for(var a,r,d;e=e.__;)if((a=e.__c)&&!a.__)try{if((r=a.constructor)&&r.getDerivedStateFromError!=null&&(a.setState(r.getDerivedStateFromError(t)),d=a.__d),a.componentDidCatch!=null&&(a.componentDidCatch(t,o||{}),d=a.__d),d)return a.__E=a}catch(l){t=l}throw t}},he=0,at=function(t){return t!=null&&t.constructor===void 0},B.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=P({},this.state),typeof t=="function"&&(t=t(P({},n),this.props)),t&&P(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),re(this))},B.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),re(this))},B.prototype.render=C,I=[],me=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,be=function(t,e){return t.__v.__b-e.__v.__b},q.__r=0,ee=Math.random().toString(8),W="__d"+ee,O="__a"+ee,ve=/(PointerCapture)$|Capture$/i,oe=0,te=fe(!1),ne=fe(!0),ye=0;var U,m,se,Ne,Y=0,Re=[],v=p,Ae=v.__b,Ue=v.__r,De=v.diffed,Me=v.__c,Oe=v.unmount,$e=v.__;function Q(t,e){v.__h&&v.__h(m,t,Y||e),Y=0;var n=m.__H||(m.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function $(t){return Y=1,gt(Be,t)}function gt(t,e,n){var o=Q(U++,2);if(o.t=t,!o.__c&&(o.__=[n?n(e):Be(void 0,e),function(l){var s=o.__N?o.__N[0]:o.__[0],c=o.t(s,l);s!==c&&(o.__N=[c,o.__[1]],o.__c.setState({}))}],o.__c=m,!m.__f)){var a=function(l,s,c){if(!o.__c.__H)return!0;var _=!1,h=o.__c.props!==l;if(o.__c.__H.__.some(function(u){if(u.__N){_=!0;var b=u.__[0];u.__=u.__N,u.__N=void 0,b!==u.__[0]&&(h=!0)}}),r){var i=r.call(this,l,s,c);return _?i||h:i}return!_||h};m.__f=!0;var r=m.shouldComponentUpdate,d=m.componentWillUpdate;m.componentWillUpdate=function(l,s,c){if(this.__e){var _=r;r=void 0,a(l,s,c),r=_}d&&d.call(this,l,s,c)},m.shouldComponentUpdate=a}return o.__N||o.__}function Le(t,e){var n=Q(U++,4);!v.__s&&je(n.__H,e)&&(n.__=t,n.u=e,m.__h.push(n))}function Fe(t){return Y=5,pt(function(){return{current:t}},[])}function pt(t,e){var n=Q(U++,7);return je(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function We(t){var e=m.context[t.__c],n=Q(U++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(m)),e.props.value):t.__}function ft(){for(var t;t=Re.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(X),e.__h.some(le),e.__h=[]}catch(n){e.__h=[],v.__e(n,t.__v)}}}v.__b=function(t){m=null,Ae&&Ae(t)},v.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),$e&&$e(t,e)},v.__r=function(t){Ue&&Ue(t),U=0;var e=(m=t.__c).__H;e&&(se===m?(e.__h=[],m.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(X),e.__h.some(le),e.__h=[],U=0)),se=m},v.diffed=function(t){De&&De(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(Re.push(e)!==1&&Ne===v.requestAnimationFrame||((Ne=v.requestAnimationFrame)||ht)(ft)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),se=m=null},v.__c=function(t,e){e.some(function(n){try{n.__h.some(X),n.__h=n.__h.filter(function(o){return!o.__||le(o)})}catch(o){e.some(function(a){a.__h&&(a.__h=[])}),e=[],v.__e(o,n.__v)}}),Me&&Me(t,e)},v.unmount=function(t){Oe&&Oe(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(o){try{X(o)}catch(a){e=a}}),n.__H=void 0,e&&v.__e(e,n.__v))};var He=typeof requestAnimationFrame=="function";function ht(t){var e,n=function(){clearTimeout(o),He&&cancelAnimationFrame(e),setTimeout(t)},o=setTimeout(n,35);He&&(e=requestAnimationFrame(n))}function X(t){var e=m,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),m=e}function le(t){var e=m;t.__c=t.__(),m=e}function je(t,e){return!t||t.length!==e.length||e.some(function(n,o){return n!==t[o]})}function Be(t,e){return typeof e=="function"?e(t):e}var ze="(prefers-color-scheme: dark)";function mt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function bt(t){let e=t;for(;e;){let n=mt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(ze).matches?"dark":"light"}function Ve(t){let[e,n]=$("light");return Le(()=>{let o=t.current;if(!o)return;let a=()=>n(bt(o.parentElement||o));a();let r=new MutationObserver(a),d={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};r.observe(document.documentElement,d),document.body&&r.observe(document.body,d);let l=window.matchMedia?window.matchMedia(ze):null;return l&&l.addEventListener("change",a),()=>{r.disconnect(),l&&l.removeEventListener("change",a)}},[t]),e}var S={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",badgeRadius:13,badgeRadiusActive:15,badgeOnEdgeMin:60},vt=`
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
`,qe=`
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
`,yt=`
  .dg-diagram { ${vt} }
  @media (prefers-color-scheme: dark) {
    .dg-diagram:not([data-dg-theme="light"]) { ${qe} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${qe} }

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
`,Ke="dg-diagram-styles";function Ge(){if(typeof document=="undefined"||document.getElementById(Ke))return;let t=document.createElement("style");t.id=Ke,t.textContent=yt,document.head.appendChild(t)}var xt=0,Ht=Array.isArray;function g(t,e,n,o,a,r){e||(e={});var d,l,s=e;if("ref"in s)for(l in s={},e)l=="ref"?d=e[l]:s[l]=e[l];var c={type:t,props:s,key:n,ref:d,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--xt,__i:-1,__u:0,__source:a,__self:r};if(typeof t=="function"&&(d=t.defaultProps))for(l in d)s[l]===void 0&&(s[l]=d[l]);return p.vnode&&p.vnode(c),c}var Ye=Ee({markerPrefix:"dg-arrow"});function Tt(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function wt(t,e="Edge"){for(let n=1;n<t.length;n++){let[o,a]=t[n-1],[r,d]=t[n];if(o!==r&&a!==d)throw new Error(`${e}: diagonal segment (${o},${a})\u2192(${r},${d}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function J({x:t,y:e,w:n,h:o,label:a,sub:r,variant:d="default",small:l}){let s=d.startsWith("solid-"),c=r?e+o/2-6:e+o/2+5;return g("g",{class:`dg-box dg-box-${d}${s?" dg-box-solid":""}`,children:[g("rect",{x:t,y:e,width:n,height:o,rx:S.boxRadius,"stroke-width":S.boxStroke}),g("text",{x:t+n/2,y:c,"text-anchor":"middle",class:l?"dg-label dg-label-sm":"dg-label dg-label-bold",children:a}),r?g("text",{x:t+n/2,y:c+(l?14:18),"text-anchor":"middle",class:"dg-label dg-label-sub",children:r}):null]})}function Qe({points:t,dashed:e,active:n,dir:o,id:a}){wt(t,a?`Edge ${a}`:"Edge");let{markerPrefix:r}=We(Ye),d=`${r}-${n?"active":"default"}`,l=o&&o!=="none";return g("polyline",{class:`dg-edge${n?" dg-edge-active":""}`,points:Tt(t),fill:"none","stroke-width":n?S.edgeStrokeActive:S.edgeStroke,"stroke-dasharray":e?S.edgeDash:void 0,"marker-end":l?`url(#${d})`:void 0,"marker-start":o==="both"?`url(#${d})`:void 0})}function Je({id:t,label:e,name:n,x:o,y:a,active:r,pressed:d,onSelect:l,onHover:s}){let c=(_,h)=>{_.pointerType==="mouse"&&s&&s(h)};return g("g",{class:`dg-badge${r?" dg-badge-active":""}`,role:"button",tabindex:"0","aria-pressed":d,"aria-label":n?`${t}: ${n}`:t,onClick:()=>l(t),onPointerEnter:_=>c(_,t),onPointerLeave:_=>c(_,null),onKeyDown:_=>{(_.key==="Enter"||_.key===" ")&&(_.preventDefault(),l(t))},children:[g("circle",{cx:o,cy:a,r:r?S.badgeRadiusActive:S.badgeRadius,"stroke-width":"1.5"}),g("text",{x:o,y:a+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function Ze(){let[t,e]=$(null),[n,o]=$(null);return{shownId:n||t,pinnedId:t,select:r=>e(d=>d===r?null:r),setHover:o}}function Pt({tone:t="neutral",id:e,text:n}){return g("span",{class:`dg-chip dg-chip-${t}`,children:[g("span",{class:"dg-chip-id",children:e}),n?g("span",{class:"dg-chip-desc",children:n}):null]})}function et({id:t,title:e,desc:n,chips:o,hint:a,children:r}){return g("div",{class:"dg-panel","aria-live":"polite",children:e?g(C,{children:[g("div",{class:"dg-panel-title",children:[t?g(C,{children:[t," \u2014 "]}):null,e]}),n?g("div",{class:"dg-panel-desc",children:n}):null,o&&o.length?g("div",{class:"dg-chips",children:o.map(d=>g(Pt,{...d},`${d.tone}-${d.id}`))}):null,r]}):g("div",{class:"dg-hint",children:a})})}function Xe({id:t,active:e}){return g("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:g("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}function tt({id:t,viewH:e,viewW:n=960,label:o,footer:a,children:r}){Ge();let d=Fe(null),l=Ve(d),s=`dg-arrow-${t}`;return g("div",{class:"dg-diagram","data-dg-theme":l,ref:d,children:[g(Ye.Provider,{value:{markerPrefix:s},children:g("svg",{viewBox:`0 0 ${n} ${e}`,role:"group","aria-label":o,children:[g("defs",{children:[g(Xe,{id:`${s}-default`}),g(Xe,{id:`${s}-active`,active:!0})]}),r]})}),a]})}var ce={"UC-1":"Provision","UC-2":"Authenticate","UC-3":"Task exchange","IP-1":"Operator I/O","IP-2":"Docker control plane","IP-3":"Exec channel","IP-4":"Kernel boundary","IP-4a":"Container \u2192 Sentry","IP-4b":"Sentry \u2192 host kernel","IP-5":"Container filesystem","IP-6":"Network egress",T1a:"outbound POST",T1b:"DNS tunnelling",T1d:"ambient credential",T1e:"terminal query \u2192 stdin","T2a\u2013d":"supply chain",T2c:"install-time hooks",T3b:"irreversible remote action",T3c:"destructive retry loop","T4a\u2013c":"lateral movement",T5b:"PATH / build-file backdoor",T5d:"reused sandbox state",T6a:"shared kernel surface",T6b:"runc binary overwrite",T6c:"leaked-fd escape",T6d:"privileged / misconfigured",T6e:"shared-hardware side channels",T6f:"no cgroup caps","T6g\u2013j":"Sentry / Gofer residual",T6k:"platform exposure",T6l:"env flips runtime","T7a\u2013c":"no PID / mem / disk caps",T7d:"Sentry DoS",T7e:"unbounded read_until \xB7 no exec timeout",T8b:"unscoped docker.from_env()",T9a:"marker-driven state",T9b:"unsupervised tool loop",T9c:"no per-action gate",T9d:"raw print() to terminal",T9e:"CLI output is the prompt"},nt={"IP-1":{name:"Operator I/O",code:"gateway.ask_user() \u2192 interrupt(), answered by input() in main.py; gateway.notify_user() \u2192 print()",desc:"The sole human-in-the-loop gate, at turn granularity only. Agent text reaches the terminal emulator unfiltered \u2014 the _clean regex runs on login output alone.",uc:["UC-1","UC-2","UC-3"],open:["T9c","T9d","T9e","T1e"],closed:[]},"IP-2":{name:"Docker control plane",code:"container.client() \u2014 module-level docker.from_env(); create(), find(), containers.get() in nodes 2\u20134, 6",desc:"Full daemon access from the harness process. No socket is mounted into the container and the agent cannot spawn containers, so only the orchestrator side is live.",uc:["UC-1","UC-2","UC-3"],open:["T8b","T5d"],closed:[]},"IP-3":{name:"Exec channel",code:"ExecSession (exec_create stdin+tty, attached socket) for the login relay; ct.exec() \u2192 exec_run(demux=False) for auth status and claude -p",desc:"Free text in both directions. Login transitions on literal markers; read_until grows its buffer unbounded; the timeout ct.exec() accepts is never passed on.",uc:["UC-2","UC-3"],open:["T9a","T9b","T7e","T3c"],closed:[]},"IP-5":{name:"Container filesystem",code:"create(): read_only=False; no volumes, mounts, or tmpfs",desc:"The whole rootfs is writable \u2014 including the CLI binary on PATH and the OAuth session claude auth login persists \u2014 and node 2 reuses it across runs. No host path is mounted, so the blast radius stops at the container.",uc:["UC-3"],open:["T1d","T2c","T5b","T5d","T7a\u2013c"],closed:[]},"IP-6":{name:"Network egress",code:'create(): network_mode="bridge"; no egress or DNS policy',desc:"The CLI reaches the Anthropic API because everything is reachable: registries, link-local metadata, host services, and every other agent container on the same default bridge.",uc:["UC-2","UC-3"],open:["T1a","T1b","T2a\u2013d","T4a\u2013c","T3b"],closed:[]}},qt={runc:{...nt,"IP-4":{name:"Kernel boundary",code:'CONTAINER_RUNTIME=runc in the environment; cap_drop=["ALL"], security_opt=["no-new-privileges"] still apply',desc:"Raw syscalls to the shared host kernel. cap_drop and no mounts still close the misconfiguration class, but every kernel and runc CVE is one hop from the orchestrator.",uc:["UC-1","UC-3"],open:["T6a","T6b","T6c","T6e","T6f","T6l"],closed:["T6d"]}},runsc:{...nt,"IP-4a":{name:"Container \u2192 Sentry",code:'create(): runtime=config.CONTAINER_RUNTIME (os.getenv default "runsc"), cap_drop=["ALL"], security_opt=["no-new-privileges"]',desc:"Syscalls are intercepted by gVisor\u2019s userspace Sentry before they would ever reach the host kernel. Closes the shared-kernel and runc-CVE cases; the kwargs close the misconfiguration class.",uc:["UC-1","UC-3"],open:["T6g\u2013j","T6l"],closed:["T6a","T6b","T6c","T6d"]},"IP-4b":{name:"Sentry \u2192 host kernel",code:"whatever the daemon\u2019s runsc platform is (ptrace / KVM / systrap); no mem_limit, pids_limit, or CPU quota",desc:"The Sentry\u2019s own narrow, seccomp-filtered call set. A Sentry compromise lands here, contained to that process \u2014 but nothing caps what it may consume.",uc:["UC-1","UC-3"],open:["T6e","T6f","T6k","T7a\u2013c","T7d"],closed:[]}}},H={N1:{name:"select_agent",desc:'ask_user("Which agent do you want to work with?") \u2192 interrupt(). The name, lower-cased and hyphenated, becomes the container name via CONTAINER_PREFIX.',ip:["IP-1"],t:[]},N2:{name:"check_agent",desc:"ct.find(name) \u2192 containers.get(). If the container exists, ensure_running() (re)starts it and the graph skips creation \u2014 the previous run\u2019s login and writes come along.",ip:["IP-2"],t:["T5d"]},N3:{name:"create_container",desc:'ct.create(name) \u2192 containers.run(image, runtime=CONTAINER_RUNTIME, network_mode="bridge", read_only=False, security_opt=["no-new-privileges"], cap_drop=["ALL"]). notify_user() prints "runtime=runsc" as a literal, whatever the env resolved.',ip:["IP-2","IP-4","IP-5","IP-6"],t:["T6l","T7a\u2013c","T8b"]},N4:{name:"auth_gateway (loop)",desc:'check: is_authenticated() runs claude auth status; else start_login() opens a TTY ExecSession on claude auth login and read_until("Paste code here"). relay: interrupt() with that output as the prompt, then finish_login() writes the reply back, read_until("Invalid code"), and sets authenticated from exit code + auth status. Capped at MAX_AUTH_ATTEMPTS.',ip:["IP-1","IP-2","IP-3"],t:["T9a","T9e","T7e","T1d"]},N5:{name:"ask_task",desc:'ask_user("\u2026 What do you want to do?") \u2192 interrupt(). Seeds the transcript with the first user turn.',ip:["IP-1"],t:[]},N6:{name:"exchange (loop)",desc:'agent turn: send_prompt() runs claude -p --output-format text via exec_run, piping the last user message through sh -c "printf %s \u2026 | claude -p"; the answer goes to notify_user() \u2192 print(). user turn: interrupt(); an EXIT_WORDS entry sets done. Only the operator can end the loop.',ip:["IP-1","IP-3","IP-5","IP-6"],t:["T9b","T9c","T9d","T1e","T3c"]}};var Ct=450,R={start:{x:20,y:60,w:60,h:60},N1:{x:110,y:60,w:150,h:60},N2:{x:290,y:60,w:150,h:60},N3:{x:470,y:60,w:150,h:60},N4:{x:650,y:60,w:150,h:60},N5:{x:650,y:250,w:150,h:60},N6:{x:470,y:250,w:150,h:60},end:{x:470,y:390,w:150,h:44}},rt=[{id:"start",points:[[80,90],[110,90]],dir:"end"},{id:"1-2",points:[[260,90],[290,90]],dir:"end"},{id:"2-3",points:[[440,90],[470,90]],dir:"end",label:"no container",at:[455,138],anchor:"middle"},{id:"2-4",points:[[365,120],[365,160],[690,160],[690,120]],dir:"end",label:"container exists \u2192 reattach",at:[527,174],anchor:"middle"},{id:"3-4",points:[[620,90],[650,90]],dir:"end"},{id:"4-4",points:[[770,60],[770,30],[700,30],[700,60]],dir:"end",label:"not authenticated \xB7 attempts < MAX",at:[735,22],anchor:"middle"},{id:"4-5",points:[[760,120],[760,250]],dir:"end",label:"authenticated",at:[770,220],anchor:"start"},{id:"4-end",points:[[800,90],[840,90],[840,412],[620,412]],dir:"end",label:"attempts \u2265 MAX_AUTH_ATTEMPTS",at:[850,300],anchor:"start",vertical:!0},{id:"5-6",points:[[650,280],[620,280]],dir:"end"},{id:"6-6",points:[[470,262],[440,262],[440,290],[470,290]],dir:"end",label:"agent \u2194 user turn",at:[432,305],anchor:"end"},{id:"6-end",points:[[545,310],[545,390]],dir:"end",label:"done (/quit)",at:[555,355],anchor:"start"}],It={N1:"interrupt() \xB7 name",N2:"find() \xB7 ensure_running()",N3:"containers.run(runsc\u2026)",N4:"interrupt() \xB7 OAuth code",N5:"interrupt() \xB7 first task",N6:"interrupt() \xB7 claude -p"};function St({text:t,at:e,anchor:n="middle",vertical:o}){let[a,r]=e;return g("text",{x:a,y:r,"text-anchor":n,class:"dg-label dg-label-sub",transform:o?`rotate(-90 ${a} ${r})`:void 0,children:t})}function _e({id:t="graph"}){let{shownId:e,pinnedId:n,select:o,setHover:a}=Ze(),r=e?H[e]:null,d=r?[...r.ip.map(s=>({tone:"info",id:s,text:ce[s]})),...r.t.map(s=>({tone:"danger",id:s,text:ce[s]}))]:[],l=Object.keys(H);return g(tt,{id:t,viewH:Ct,label:"LangGraph state machine of the minimal harness: six nodes, two self-loops, four interrupt points",footer:g(et,{id:e,title:r==null?void 0:r.name,desc:r==null?void 0:r.desc,chips:d,hint:"Hover or click a numbered node for what it calls, the interaction points it touches, and the threat cases that live there."}),children:[g(J,{...R.start,label:"START",variant:"solid-dark",small:!0}),g(J,{...R.end,label:"END",variant:"solid-dark",small:!0}),l.map(s=>g(J,{...R[s],label:H[s].name.replace(" (loop)",""),sub:It[s]},s)),rt.map(s=>g(Qe,{id:s.id,points:s.points,dir:s.dir},s.id)),rt.filter(s=>s.label).map(s=>g(St,{text:s.label,at:s.at,anchor:s.anchor,vertical:s.vertical},`${s.id}-label`)),l.map(s=>g(Je,{id:s,label:s.replace("N",""),name:H[s].name,x:R[s].x,y:R[s].y,active:e===s,pressed:n===s,onSelect:o,onHover:a},s))]})}var Et={"island-graph":{id:"graph"}};function ot(){for(let[t,e]of Object.entries(Et)){let n=document.getElementById(t);n&&Se(g(_e,{...e}),n)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",ot):ot();})();
