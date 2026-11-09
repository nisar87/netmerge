// Render the existing vector brand mark at native Windows icon sizes.
// Only needed when the icon changes; normal Go builds use committed .syso files.
import { chromium } from '@playwright/test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
const source=new URL('../../ui/assets/netmerge.svg',import.meta.url);
const output=new URL('../../assets/windows/netmerge.ico',import.meta.url);
const sizes=[16,20,24,32,48,64,128,256],images=[];
const svg=await readFile(source,'utf8');
const browser=await chromium.launch(process.env.NETMERGE_BROWSER_EXECUTABLE?{executablePath:process.env.NETMERGE_BROWSER_EXECUTABLE}:{});
try {
  const page=await browser.newPage({viewport:{width:256,height:256},deviceScaleFactor:1});
  await page.setContent(`<style>html,body{margin:0;background:transparent}svg{display:block;width:100vw;height:100vh}</style>${svg}`);
  for(const size of sizes){
    await page.setViewportSize({width:size,height:size});
    images.push(await page.screenshot({type:'png',omitBackground:true}));
  }
}finally{await browser.close();}
const directory=Buffer.alloc(6+sizes.length*16);
directory.writeUInt16LE(1,2);directory.writeUInt16LE(sizes.length,4);
let offset=directory.length;
sizes.forEach((size,i)=>{
  const p=6+i*16;
  directory[p]=directory[p+1]=size===256?0:size;
  directory.writeUInt16LE(1,p+4);directory.writeUInt16LE(32,p+6);
  directory.writeUInt32LE(images[i].length,p+8);directory.writeUInt32LE(offset,p+12);
  offset+=images[i].length;
});
await mkdir(new URL('../../assets/windows/',import.meta.url),{recursive:true});
await writeFile(output,Buffer.concat([directory,...images]));
console.log(`Created ${output.pathname} with ${sizes.join(', ')}px icons.`);
