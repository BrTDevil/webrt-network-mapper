import json
# Fiecare icon: 48x48, stroke=currentColor. F = umplere discretă cu accent.
F='fill="currentColor" fill-opacity=".15"'
S='fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"'
def g(body): return f'<g {S}>{body}</g>'
icons = [
 # Rețea
 ("internet","Internet / ISP","Rețea", g(f'<circle cx="24" cy="24" r="17" {F}/><ellipse cx="24" cy="24" rx="7" ry="17"/><path d="M7 24h34M10 15h28M10 33h28"/>')),
 ("modem","Modem","Rețea", g(f'<rect x="6" y="20" width="36" height="14" rx="3" {F}/><path d="M12 27h2M18 27h2M24 27h2"/><path d="M34 20V9M30 12l4-4 4 4"/><path d="M11 34v4M37 34v4"/>')),
 ("router","Router","Rețea", g(f'<rect x="5" y="24" width="38" height="12" rx="3" {F}/><path d="M11 30h2M17 30h2M23 30h2M29 30h2"/><path d="M12 24l-3-13M36 24l3-13"/><path d="M19 15a7 7 0 0 1 10 0M16 11a12 12 0 0 1 16 0"/>')),
 ("router_ap","Router ca AP / amplificator","Rețea", g(f'<rect x="7" y="26" width="34" height="11" rx="3" {F}/><path d="M13 31.5h2M19 31.5h2"/><path d="M28 31.5h8"/><path d="M24 26v-6"/><path d="M18 16a8.5 8.5 0 0 1 12 0M14 12a14 14 0 0 1 20 0"/><path d="M33 20l4 3-4 3" /><path d="M15 20l-4 3 4 3"/>')),
 ("ap","Access Point (tavan)","Rețea", g(f'<ellipse cx="24" cy="30" rx="16" ry="7" {F}/><circle cx="24" cy="30" r="2"/><path d="M17 17a10 10 0 0 1 14 0M12 12a17 17 0 0 1 24 0"/>')),
 ("switch","Switch","Rețea", g(f'<rect x="4" y="17" width="40" height="14" rx="2" {F}/><path d="M9 22v4M13 22v4M17 22v4M21 22v4M25 22v4M29 22v4M33 22v4"/><circle cx="39" cy="24" r="1.2"/>')),
 ("patch_panel","Patch panel","Rețea", g(f'<rect x="3" y="18" width="42" height="12" rx="1.5" {F}/><path d="M3 18v12M45 18v12"/><rect x="8" y="21.5" width="4" height="5"/><rect x="15" y="21.5" width="4" height="5"/><rect x="22" y="21.5" width="4" height="5"/><rect x="29" y="21.5" width="4" height="5"/><rect x="36" y="21.5" width="4" height="5"/>')),
 ("rack","Rack","Rețea", g(f'<rect x="11" y="4" width="26" height="40" rx="2" {F}/><path d="M15 11h18M15 18h18M15 25h18M15 32h18"/><circle cx="31" cy="14.5" r=".8"/><circle cx="31" cy="21.5" r=".8"/><path d="M14 44v-3M34 44v-3"/>')),
 ("firewall","Firewall","Rețea", g(f'<rect x="6" y="10" width="36" height="28" rx="2" {F}/><path d="M6 19h36M6 28h36M18 10v9M30 10v9M12 19v9M24 19v9M36 19v9M18 28v10M30 28v10"/>')),
 # Servere / stații
 ("server","Server","Servere / stații", g(f'<rect x="10" y="5" width="28" height="11" rx="2" {F}/><rect x="10" y="18.5" width="28" height="11" rx="2" {F}/><rect x="10" y="32" width="28" height="11" rx="2" {F}/><path d="M15 10.5h8M15 24h8M15 37.5h8"/><circle cx="32" cy="10.5" r="1.2"/><circle cx="32" cy="24" r="1.2"/><circle cx="32" cy="37.5" r="1.2"/>')),
 ("nas","NAS","Servere / stații", g(f'<rect x="12" y="6" width="24" height="36" rx="3" {F}/><rect x="16" y="11" width="16" height="7" rx="1"/><rect x="16" y="21" width="16" height="7" rx="1"/><circle cx="19" cy="36" r="1.2"/><circle cx="24" cy="36" r="1.2"/>')),
 ("pc","PC","Servere / stații", g(f'<rect x="4" y="9" width="28" height="20" rx="2" {F}/><path d="M14 36h8M18 29v7"/><rect x="35" y="9" width="9" height="28" rx="1.5"/><path d="M38 14h3M38 18h3"/>')),
 ("laptop","Laptop","Servere / stații", g(f'<rect x="9" y="10" width="30" height="20" rx="2" {F}/><path d="M4 34h40l-3 4H7z"/>')),
 ("monitor","Monitor","Servere / stații", g(f'<rect x="5" y="8" width="38" height="25" rx="2" {F}/><path d="M18 40h12M24 33v7"/>')),
 ("terminal","Terminal / stație de lucru","Servere / stații", g(f'<rect x="7" y="7" width="34" height="23" rx="2" {F}/><path d="M13 15l5 4-5 4M21 23h8"/><path d="M10 41h28l-3-6H13z"/>')),
 ("pc_bd","Computer bază de date produse","Servere / stații", g(f'<rect x="3" y="8" width="26" height="19" rx="2" {F}/><path d="M12 34h8M16 27v7"/><ellipse cx="37" cy="14" rx="8" ry="3.5"/><path d="M29 14v20c0 2 3.6 3.5 8 3.5s8-1.5 8-3.5V14"/><path d="M29 20.5c0 2 3.6 3.5 8 3.5s8-1.5 8-3.5M29 27c0 2 3.6 3.5 8 3.5s8-1.5 8-3.5"/>')),
 # POS
 ("casa_de_marcat","Casă de marcat","POS / comercial", g(f'<path d="M6 28h36v12H6z" {F}/><path d="M10 28l3-10h22l3 10"/><rect x="16" y="7" width="16" height="8" rx="1"/><path d="M24 15v3M12 34h24"/><path d="M17 23h2M23 23h2M29 23h2"/>')),
 ("terminal_pos","Terminal POS / card","POS / comercial", g(f'<rect x="13" y="4" width="22" height="40" rx="4" {F}/><rect x="17" y="8" width="14" height="9" rx="1"/><path d="M18 23h2M23 23h2M28 23h2M18 28h2M23 28h2M28 28h2M18 33h2M23 33h2M28 33h2"/><path d="M13 39h22"/>')),
 ("imprimanta_bonuri","Imprimantă bonuri","POS / comercial", g(f'<rect x="8" y="20" width="32" height="20" rx="3" {F}/><path d="M16 20V6h16v14"/><path d="M19 11h10M19 15h7"/><circle cx="33" cy="30" r="1.5"/>')),
 ("imprimanta_bucatarie","Imprimantă bucătărie","POS / comercial", g(f'<rect x="8" y="22" width="32" height="18" rx="3" {F}/><path d="M16 22v-6h16v6"/><path d="M17 10c0-3 3-3 3-6M24 10c0-3 3-3 3-6M31 10c0-3 3-3 3-6" stroke-width="2"/><circle cx="33" cy="31" r="1.5"/>')),
 ("tableta_ospatar","Tabletă ospătar","POS / comercial", g(f'<rect x="8" y="6" width="32" height="36" rx="4" {F}/><path d="M13 13h16M13 19h12M13 25h14"/><path d="M31 31l3 3 5-6"/>')),
 ("terminal_ospatar","Terminal ospătar (handheld)","POS / comercial", g(f'<path d="M14 5h20a3 3 0 0 1 3 3v32a3 3 0 0 1-3 3H14a3 3 0 0 1-3-3V8a3 3 0 0 1 3-3z" {F}/><rect x="15" y="9" width="18" height="13" rx="1"/><path d="M18 13h8M18 17h12"/><path d="M17 27h3M22.5 27h3M28 27h3M17 32h3M22.5 32h3M28 32h3"/><path d="M19 38h10"/><path d="M34 2l3 3" stroke-width="2"/>')),
 ("kiosk","Kiosk self-order","POS / comercial", g(f'<rect x="12" y="4" width="24" height="26" rx="2" {F}/><path d="M17 11h14M17 16h10"/><rect x="18" y="21" width="12" height="5" rx="1"/><path d="M20 30v10M28 30v10M14 43h20"/>')),
 ("scanner_coduri","Scanner coduri de bare","POS / comercial", g(f'<path d="M8 10h22l6 12-8 4-4-6H14z" {F}/><path d="M20 26l-4 16h8l4-16"/><path d="M40 10v14" stroke-dasharray="2 2"/>')),
 ("cantar","Cântar","POS / comercial", g(f'<path d="M6 36h36l-3 6H9z" {F}/><path d="M10 30h28v6H10z"/><path d="M24 30V18"/><path d="M12 18h24l-4-6H16z"/><rect x="20" y="32" width="8" height="2.5" rx=".5"/>')),
 # Periferice
 ("imprimanta","Imprimantă","Periferice", g(f'<path d="M14 17V6h20v11"/><rect x="6" y="17" width="36" height="17" rx="3" {F}/><path d="M14 28h20v14H14z"/><path d="M18 33h12M18 37h8"/><circle cx="36" cy="22" r="1.3"/>')),
 ("camera","Cameră IP","Periferice", g(f'<path d="M6 14l28-5 3 12-28 5z" {F}/><circle cx="31" cy="16" r="2.5"/><path d="M20 24l3 9M16 42h14l-5-9h-4z"/>')),
 ("camera_dome","Cameră dome","Periferice", g(f'<path d="M8 14h32" /><path d="M11 14a13 13 0 0 0 26 0" {F}/><circle cx="24" cy="21" r="4.5"/><circle cx="24" cy="21" r="1.4"/><path d="M14 10h20v4H14z"/>')),
 ("nvr","DVR / NVR","Periferice", g(f'<rect x="4" y="16" width="40" height="16" rx="2" {F}/><circle cx="12" cy="24" r="4"/><circle cx="12" cy="24" r="1.3"/><path d="M22 21h14M22 27h8"/><circle cx="39" cy="27" r="1"/>')),
 ("telefon_ip","Telefon IP","Periferice", g(f'<rect x="10" y="12" width="30" height="28" rx="3" {F}/><path d="M6 10c0-3 3-5 7-5h2v26h-2c-4 0-7-2-7-5z"/><rect x="21" y="16" width="14" height="7" rx="1"/><path d="M22 28h2M28 28h2M34 28h1M22 33h2M28 33h2M34 33h1"/>')),
 ("boxa","Boxă / sistem audio","Periferice", g(f'<rect x="12" y="4" width="24" height="40" rx="3" {F}/><circle cx="24" cy="30" r="7"/><circle cx="24" cy="30" r="2"/><circle cx="24" cy="13" r="3.5"/>')),
 ("tv","TV / Smart TV","Periferice", g(f'<rect x="3" y="9" width="42" height="26" rx="2" {F}/><path d="M16 42l4-7h8l4 7"/>')),
 ("casti","Căști Bluetooth","Periferice", g(f'<path d="M9 30v-6a15 15 0 0 1 30 0v6"/><rect x="6" y="28" width="9" height="13" rx="3" {F}/><rect x="33" y="28" width="9" height="13" rx="3" {F}/>')),
 ("mouse","Mouse / tastatură","Periferice", g(f'<rect x="15" y="6" width="18" height="34" rx="9" {F}/><path d="M24 6v12M15 18h18"/><path d="M24 40v4" stroke-width="2"/>')),
 # Mobil
 ("telefon","Telefon","Mobil", g(f'<rect x="14" y="4" width="20" height="40" rx="4" {F}/><path d="M21 9h6"/><circle cx="24" cy="38.5" r="1.4"/>')),
 ("tableta","Tabletă","Mobil", g(f'<rect x="7" y="6" width="34" height="36" rx="4" {F}/><circle cx="24" cy="37.5" r="1.4"/><path d="M11 11h26v21H11z" stroke-width="1.5"/>')),
 ("ceas","Ceas / brățară","Mobil", g(f'<path d="M17 12l2-7h10l2 7M17 36l2 7h10l2-7"/><rect x="13" y="12" width="22" height="24" rx="6" {F}/><path d="M24 18v6l4 3"/>')),
 # Infrastructură
 ("ups","UPS","Infrastructură", g(f'<rect x="12" y="5" width="24" height="38" rx="3" {F}/><path d="M26 11l-6 10h8l-6 10"/><path d="M18 37h12"/>')),
 ("priza_retea","Priză rețea (perete)","Infrastructură", g(f'<rect x="8" y="8" width="32" height="32" rx="4" {F}/><path d="M17 18h14v10h-3v3h-8v-3h-3z"/><path d="M20 22v2M24 22v2M28 22v2"/>')),
 ("priza_electrica","Priză electrică","Infrastructură", g(f'<rect x="8" y="8" width="32" height="32" rx="4" {F}/><circle cx="24" cy="24" r="11"/><circle cx="19.5" cy="24" r="1.8"/><circle cx="28.5" cy="24" r="1.8"/><path d="M24 13v2.5M24 32.5V35"/>')),
 ("tablou_electric","Tablou electric","Infrastructură", g(f'<rect x="8" y="4" width="32" height="40" rx="2" {F}/><path d="M13 12h22v10H13z"/><path d="M17 12v10M21 12v10M25 12v10M29 12v10"/><path d="M26 27l-4 7h5l-4 7" stroke-width="2"/>')),
 # Generic
 ("necunoscut","Dispozitiv necunoscut","Generic", g(f'<rect x="7" y="7" width="34" height="34" rx="6" {F}/><path d="M19 19a5 5 0 1 1 7 4.6c-1.3.6-2 1.6-2 3v2"/><circle cx="24" cy="34" r="1.3"/>')),
 ("bluetooth","Dispozitiv Bluetooth","Generic", g(f'<rect x="10" y="4" width="28" height="40" rx="14" {F}/><path d="M17 17l14 13-7 6V12l7 6-14 13"/>')),
 ("nota","Notă / etichetă","Generic", g(f'<path d="M8 6h32v26l-10 10H8z" {F}/><path d="M30 42V32h10"/><path d="M14 15h20M14 21h20M14 27h12"/>')),
]
out = {k: {"label": l, "cat": c, "svg": s} for k,l,c,s in icons}
cats = []
for _,_,c,_ in icons:
    if c not in cats: cats.append(c)
json.dump({"categories": cats, "icons": out}, open('web/icons.json','w'), ensure_ascii=False, indent=0)
print(len(out))
