package main

import (
	"html/template"
)

// parseTemplates registers all server-side HTML/HTMX templates for the Academic Sepia UI 2.0.
func (s *Server) parseTemplates() {
	layoutHTML := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Alpine Campgrounds · Wilderness Reservations & Operations</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", serif; background-color: #fbf8f3; color: #2c2825; }
        .sepia-panel { background-color: #f4efe6; border-color: #e6dfd5; }
        .sepia-card { background-color: #ffffff; border-color: #e6dfd5; }
        .sepia-btn-primary { background-color: #8c5e38; color: #ffffff; }
        .sepia-btn-primary:hover { background-color: #724b2c; }
        .sepia-btn-forest { background-color: #2d5a27; color: #ffffff; }
        .sepia-btn-forest:hover { background-color: #21431d; }
        .sepia-pill { background-color: #eee8dc; color: #554b42; }
    </style>
</head>
<body class="h-screen flex flex-col overflow-hidden text-stone-800">
    <!-- Academic Sepia Header -->
    <header class="h-14 border-b border-[#e6dfd5] bg-[#f4efe6] px-6 flex items-center justify-between shadow-sm flex-shrink-0">
        <div class="flex items-center space-x-3">
            <span class="text-lg font-bold tracking-tight text-[#2c2825] font-serif">🌲 Alpine National Park</span>
            <span class="text-[11px] uppercase tracking-wider px-2 py-0.5 rounded bg-emerald-100 text-emerald-800 font-mono font-semibold">Campgrounds & Backcountry</span>
            <span class="text-xs text-stone-500 font-mono">Mount Rainier District</span>
        </div>
        
        <!-- Intent Pills Header Bar -->
        <div class="hidden lg:flex items-center space-x-2">
            <button onclick="switchTab('campsites'); inspectDomain('lease')" class="px-2.5 py-1 text-[11px] font-mono rounded bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-800 border border-[#ded5c7] flex items-center space-x-1 shadow-sm">
                <span>⚡</span><span>Site Holds</span>
            </button>
            <button onclick="switchTab('rothermel-telemetry'); inspectDomain('shield')" class="px-2.5 py-1 text-[11px] font-mono rounded bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-800 border border-[#ded5c7] flex items-center space-x-1 shadow-sm">
                <span>🛡️</span><span>Ranger Station</span>
            </button>
            <button onclick="switchTab('fleet-gate')" class="px-2.5 py-1 text-[11px] font-mono rounded bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-800 border border-[#ded5c7] flex items-center space-x-1 shadow-sm">
                <span>🚗</span><span>Park Gate</span>
            </button>
            <button onclick="switchTab('wild-pms')" class="px-2.5 py-1 text-[11px] font-mono rounded bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-800 border border-[#ded5c7] flex items-center space-x-1 shadow-sm">
                <span>🌲</span><span>Facilities & ICS</span>
            </button>
            <button onclick="switchTab('backcountry-lockers')" class="px-2.5 py-1 text-[11px] font-mono rounded bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-800 border border-[#ded5c7] flex items-center space-x-1 shadow-sm">
                <span>🏔️</span><span>Backcountry & Gear</span>
            </button>
            <button onclick="switchTab('rothermel-telemetry')" class="px-2.5 py-1 text-[11px] font-mono rounded bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-800 border border-[#ded5c7] flex items-center space-x-1 shadow-sm">
                <span>🔥</span><span>Wildfire & Weather</span>
            </button>
        </div>

        <!-- Persona Quick Switcher & User Menu -->
        <div class="flex items-center space-x-3">
            <div class="flex items-center space-x-1 text-xs">
                <span class="text-stone-500 font-mono text-[11px]">Persona:</span>
                <a href="/switch-persona?user=alice" class="px-2 py-0.5 rounded bg-[#eee8dc] hover:bg-[#e2dacf] font-mono {{if .User}}{{if eq .User.Email "alice@camping.local"}}border border-stone-600 font-bold bg-[#ded7c7]{{end}}{{end}}">Alice</a>
                <a href="/switch-persona?user=bob" class="px-2 py-0.5 rounded bg-[#eee8dc] hover:bg-[#e2dacf] font-mono {{if .User}}{{if eq .User.Email "bob@camping.local"}}border border-stone-600 font-bold bg-[#ded7c7]{{end}}{{end}}">Bob</a>
                <a href="/switch-persona?user=charlie" class="px-2 py-0.5 rounded bg-[#eee8dc] hover:bg-[#e2dacf] font-mono {{if .User}}{{if eq .User.Email "charlie@camping.local"}}border border-stone-600 font-bold bg-[#ded7c7]{{end}}{{end}}">Charlie</a>
            </div>

            {{if .User}}
                <div class="flex items-center space-x-2 text-xs border-l border-stone-300 pl-3">
                    <span class="font-bold text-stone-800">{{.User.FullName}}</span>
                    <a href="/logout" class="text-stone-400 hover:text-stone-700 underline text-[11px]">Logout</a>
                </div>
            {{else}}
                <div class="flex items-center space-x-2 border-l border-stone-300 pl-3">
                    <button onclick="document.getElementById('auth-modal').classList.remove('hidden')" class="text-xs sepia-btn-primary px-3 py-1 rounded">Sign In</button>
                </div>
            {{end}}
        </div>
    </header>

    <!-- 3-Panel Main Layout -->
    <div class="flex-1 flex overflow-hidden">
        
        <!-- Left Panel: Domain Navigation & Filters -->
        <aside class="w-64 border-r border-[#e6dfd5] bg-[#f4efe6] p-4 flex flex-col justify-between overflow-y-auto flex-shrink-0">
            <div class="space-y-4">
                <!-- Navigation Tabs -->
                <div>
                    <h3 class="text-[10px] font-bold uppercase tracking-wider text-stone-500 mb-2 font-mono">Campground Services</h3>
                    <div class="space-y-1 text-xs">
                        <button onclick="switchTab('campsites')" id="nav-campsites" class="w-full text-left px-3 py-2 rounded font-semibold bg-[#ded7c7] text-stone-900 flex items-center space-x-2">
                            <span>🏕️</span><span>Campsite Stays</span>
                        </button>
                        <button onclick="switchTab('fleet-gate')" id="nav-fleet-gate" class="w-full text-left px-3 py-2 rounded font-medium hover:bg-[#ded7c7] text-stone-700 flex items-center space-x-2">
                            <span>🚗</span><span>Vehicles & Gate Access</span>
                        </button>
                        <button onclick="switchTab('wild-pms')" id="nav-wild-pms" class="w-full text-left px-3 py-2 rounded font-medium hover:bg-[#ded7c7] text-stone-700 flex items-center space-x-2">
                            <span>🌲</span><span>Ranger Station & Maintenance</span>
                        </button>
                        <button onclick="switchTab('backcountry-lockers')" id="nav-backcountry-lockers" class="w-full text-left px-3 py-2 rounded font-medium hover:bg-[#ded7c7] text-stone-700 flex items-center space-x-2">
                            <span>🏔️</span><span>Backcountry Permits & Lockers</span>
                        </button>
                        <button onclick="switchTab('rothermel-telemetry')" id="nav-rothermel-telemetry" class="w-full text-left px-3 py-2 rounded font-medium hover:bg-[#ded7c7] text-stone-700 flex items-center space-x-2">
                            <span>🔥</span><span>Wildfire Safety & Weather</span>
                        </button>
                        <button hx-get="/my-bookings" hx-target="#campsite-stream" onclick="switchTab('bookings')" id="nav-bookings" class="w-full text-left px-3 py-2 rounded font-medium hover:bg-[#ded7c7] text-stone-700 flex items-center space-x-2">
                            <span>📋</span><span>My Bookings</span>
                        </button>
                    </div>
                </div>

                <!-- Campsite Discovery Filters -->
                <div class="border-t border-[#e6dfd5] pt-3">
                    <h3 class="text-[10px] font-bold uppercase tracking-wider text-stone-500 mb-2 font-mono">Inventory Filters</h3>
                    <form hx-get="/" hx-target="#campsite-cards-container" hx-trigger="change, keyup delay:200ms from:input" class="space-y-3">
                        <div>
                            <label class="block text-[11px] font-medium text-stone-600 mb-0.5">Search Query</label>
                            <input type="text" name="q" value="{{.Query}}" placeholder="River, cabin, glacier..." class="w-full text-xs px-2.5 py-1.5 rounded border border-[#e6dfd5] bg-white focus:outline-none focus:ring-1 focus:ring-stone-500">
                        </div>
                        <div>
                            <label class="block text-[11px] font-medium text-stone-600 mb-0.5">Category</label>
                            <select name="type" class="w-full text-xs px-2 py-1.5 rounded border border-[#e6dfd5] bg-white focus:outline-none">
                                <option value="all">All Categories</option>
                                <option value="tent" {{if eq .Type "tent"}}selected{{end}}>Tent Pitches</option>
                                <option value="cabin" {{if eq .Type "cabin"}}selected{{end}}>Rustic Cabins</option>
                                <option value="glamping" {{if eq .Type "glamping"}}selected{{end}}>Glamping Domes</option>
                                <option value="rv" {{if eq .Type "rv"}}selected{{end}}>RV Pads</option>
                            </select>
                        </div>
                    </form>
                </div>

                <!-- Concurrency Demonstrator Card -->
                <div class="p-3 bg-white/70 rounded border border-[#e6dfd5] text-xs space-y-2">
                    <div class="font-bold text-stone-800 flex items-center justify-between">
                        <span>⚡ Booking Collision Protection</span>
                        <span class="text-[9px] bg-stone-200 px-1 py-0.5 rounded font-mono">Instant Hold Protection</span>
                    </div>
                    <p class="text-[11px] text-stone-600 leading-tight">Simulate 2 campers attempting to book the exact same campsite slot simultaneously to verify collision lock.</p>
                    <button hx-post="/simulate-contention" hx-target="#simulation-output" class="w-full sepia-btn-primary py-1.5 rounded text-xs font-medium">Test Double-Booking Protection</button>
                    <div id="simulation-output" class="mt-2"></div>
                </div>
            </div>

            <!-- Provenance Badge -->
            <div class="text-[10px] text-stone-400 font-mono space-y-0.5 border-t border-[#e6dfd5] pt-3">
                <div>Park: Mount Rainier National Park</div>
                <div>Station: Longmire Wilderness Center</div>
                <div>Emergency Dispatch: VHF Channel 16</div>
            </div>
        </aside>

        <!-- Center Panel: Domain Workspaces -->
        <main class="flex-1 overflow-y-auto p-6" id="campsite-stream">
            
            <!-- TAB 1: CAMPSITES & STAYS -->
            <div id="workspace-campsites" class="domain-workspace space-y-4">
                <div id="campsite-cards-container">
                    {{template "campsite-cards" .}}
                </div>
            </div>

            <!-- TAB 2: CAMPER FLEET & ALPR GATEHOUSE -->
            <div id="workspace-fleet-gate" class="domain-workspace hidden space-y-6 max-w-5xl mx-auto">
                <div class="border-b border-[#e6dfd5] pb-3 flex items-center justify-between">
                    <div>
                        <h2 class="text-xl font-bold font-serif text-stone-900">Camper Vehicles & Campground Entry Gate</h2>
                        <p class="text-xs text-stone-600">Manage registered camping vehicles (including towed campers & trailers) and entry lane clearance.</p>
                    </div>
                    <span class="px-2 py-1 bg-purple-100 text-purple-900 rounded font-mono text-xs font-bold">Automated Gate Control</span>
                </div>

                <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
                    <!-- Camper Profile & Emergency Contact Form -->
                    <div class="sepia-card border rounded p-4 space-y-4 shadow-sm">
                        <div class="font-bold text-sm text-stone-900 flex items-center justify-between">
                            <span>Camper Identity & Wilderness Pass</span>
                            <span class="text-[10px] font-mono text-stone-400">E.164 Validated</span>
                        </div>
                        <form hx-post="/api/v1/profile" hx-target="#profile-save-status" class="space-y-3 text-xs">
                            <div>
                                <label class="block font-medium text-stone-700 mb-1">Full Legal Name</label>
                                <input type="text" name="full_name" value="{{if .User}}{{.User.FullName}}{{end}}" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3]">
                            </div>
                            <div class="grid grid-cols-2 gap-2">
                                <div>
                                    <label class="block font-medium text-stone-700 mb-1">Mobile Phone</label>
                                    <input type="text" name="phone" value="+1-555-0199" placeholder="+1-555-0199" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                </div>
                                <div>
                                    <label class="block font-medium text-stone-700 mb-1">Pass ID</label>
                                    <input type="text" name="wilderness_pass_id" value="PASS-NORTH-2026" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                </div>
                            </div>
                            <div class="p-2.5 bg-stone-50 border border-stone-200 rounded space-y-2">
                                <div class="font-bold text-stone-700 text-[11px]">Emergency Contact (Mandatory for Backcountry)</div>
                                <div class="grid grid-cols-2 gap-2">
                                    <input type="text" name="emergency_contact_name" value="Bob Explorer" placeholder="Contact Name" class="p-1.5 border border-[#e6dfd5] rounded bg-white text-xs">
                                    <input type="text" name="emergency_contact_phone" value="+1-555-0198" placeholder="Contact Phone" class="p-1.5 border border-[#e6dfd5] rounded bg-white text-xs font-mono">
                                </div>
                            </div>
                            <button type="submit" class="w-full sepia-btn-primary py-2 rounded text-xs font-medium">Save Camper Profile</button>
                            <div id="profile-save-status"></div>
                        </form>

                        <!-- Fleet Vehicle Management -->
                        <div class="border-t border-[#e6dfd5] pt-4 space-y-3">
                            <div class="font-bold text-sm text-stone-900 flex items-center justify-between">
                                <span>Vehicle Fleet (Towed Trailer Fallback)</span>
                                <span class="text-[10px] text-stone-500 font-mono">Multi-Vehicle</span>
                            </div>
                            <div id="fleet-vehicles-list" hx-get="/api/v1/profile" hx-trigger="load" class="space-y-2">
                                <!-- Populated dynamically -->
                                <div class="p-3 bg-white border border-[#e6dfd5] rounded text-xs flex items-center justify-between font-mono">
                                    <div><strong>WA-ALPINE1</strong> (WA) · Subaru Outback Blue <span class="px-1.5 py-0.5 bg-amber-100 text-amber-800 rounded font-bold text-[9px]">PRIMARY</span></div>
                                </div>
                                <div class="p-3 bg-white border border-[#e6dfd5] rounded text-xs flex items-center justify-between font-mono">
                                    <div><strong>WA-TRL-44</strong> (WA) · Airstream Bambi Silver <span class="px-1.5 py-0.5 bg-stone-100 text-stone-600 rounded text-[9px]">TRAILER</span></div>
                                </div>
                            </div>

                            <form hx-post="/api/v1/profile/vehicles" hx-target="#fleet-vehicles-list" class="p-3 bg-[#fbf8f3] border border-[#e6dfd5] rounded space-y-2 text-xs">
                                <div class="font-semibold text-stone-700 text-[11px]">Add Vehicle to Fleet</div>
                                <div class="grid grid-cols-2 gap-2">
                                    <input type="text" name="plate" placeholder="Plate (e.g. WA-ALPINE1)" required class="p-1.5 border border-[#e6dfd5] rounded bg-white font-mono uppercase">
                                    <input type="text" name="state" placeholder="State (e.g. WA)" required class="p-1.5 border border-[#e6dfd5] rounded bg-white font-mono uppercase">
                                </div>
                                <div class="grid grid-cols-2 gap-2">
                                    <input type="text" name="make_model" placeholder="Make / Model" class="p-1.5 border border-[#e6dfd5] rounded bg-white">
                                    <input type="text" name="color" placeholder="Color" class="p-1.5 border border-[#e6dfd5] rounded bg-white">
                                </div>
                                <div class="flex items-center space-x-4 pt-1">
                                    <label class="flex items-center space-x-1.5 text-stone-700">
                                        <input type="checkbox" name="is_primary" value="true">
                                        <span>Primary Vehicle</span>
                                    </label>
                                    <label class="flex items-center space-x-1.5 text-stone-700">
                                        <input type="checkbox" name="is_ev" value="true">
                                        <span>Electric Vehicle (EV)</span>
                                    </label>
                                </div>
                                <button type="submit" class="w-full sepia-btn-forest py-1.5 rounded text-xs font-medium">Register Vehicle to Fleet</button>
                            </form>
                        </div>
                    </div>

                    <!-- Multi-Lane ALPR Gatehouse Simulator -->
                    <div class="sepia-card border rounded p-4 space-y-4 shadow-sm flex flex-col justify-between">
                        <div class="space-y-4">
                            <div class="flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                                <div>
                                    <div class="font-bold text-sm text-stone-900">Campground Entry Gate Scanner</div>
                                    <div class="text-[11px] text-stone-500">Auto-reset timer (6s) · 3 Independent Entry Lanes</div>
                                </div>
                                <span class="px-2 py-0.5 bg-stone-100 text-stone-700 font-mono text-[10px] rounded border border-stone-300">Gate Access Control</span>
                            </div>

                            <form hx-post="/api/v1/gate/scan" hx-target="#gate-scan-output" class="space-y-3 text-xs">
                                <div>
                                    <label class="block font-medium text-stone-700 mb-1">Entry Lane Selection</label>
                                    <select name="lane" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                        <option value="STANDARD">LANE 1: Standard Vehicles (Clearance &lt; 9ft)</option>
                                        <option value="OVERSIZE_TRAILER">LANE 2: Oversize / RV / Towed Trailer (Clearance &gt; 9ft)</option>
                                        <option value="EMERGENCY_RANGER">LANE 3: Emergency Responder & Law Enforcement</option>
                                    </select>
                                </div>
                                <div class="grid grid-cols-2 gap-2">
                                    <div>
                                        <label class="block font-medium text-stone-700 mb-1">Scanned License Plate</label>
                                        <input type="text" name="plate" value="WA-ALPINE1" placeholder="WA-ALPINE1" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono uppercase font-bold">
                                    </div>
                                    <div>
                                        <label class="block font-medium text-stone-700 mb-1">State Jurisdiction</label>
                                        <input type="text" name="state" value="WA" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono uppercase">
                                    </div>
                                </div>
                                <button type="submit" class="w-full sepia-btn-primary py-2 rounded text-xs font-medium flex items-center justify-center space-x-1">
                                    <span>📸</span><span>Simulate Vehicle Scan & Open Gate</span>
                                </button>
                            </form>

                            <div id="gate-scan-output" class="p-3 bg-stone-50 border border-stone-200 rounded font-mono text-xs text-stone-500 text-center">
                                Trigger scan to test barrier arm actuation and authorization logic.
                            </div>

                            <!-- Live Barrier Status Visualizer -->
                            <div class="p-3 bg-[#fbf8f3] border border-[#e6dfd5] rounded space-y-2">
                                <div class="flex items-center justify-between text-xs font-mono">
                                    <span class="text-stone-600">Solenoid Arm Position:</span>
                                    <span id="barrier-display-badge" class="px-2 py-0.5 bg-stone-200 text-stone-800 rounded font-bold uppercase text-[10px]">GATE_CLOSED</span>
                                </div>
                                <div class="w-full bg-stone-200 h-2 rounded overflow-hidden">
                                    <div id="barrier-progress" class="bg-emerald-600 h-full w-0 transition-all duration-500"></div>
                                </div>
                                <div class="flex justify-between items-center pt-1 text-[10px] text-stone-400">
                                    <span>Auto-Reset: 6.0s Clearance Window</span>
                                    <button hx-post="/api/v1/gate/reset" hx-target="#gate-scan-output" class="text-stone-600 hover:text-stone-900 underline">Manual Reset</button>
                                </div>
                            </div>
                        </div>

                        <!-- Gate Audit Log Stream -->
                        <div class="border-t border-[#e6dfd5] pt-3">
                            <div class="flex items-center justify-between mb-2">
                                <span class="font-bold text-xs text-stone-800">Recent Gatehouse Audit Entries</span>
                                <button hx-get="/api/v1/gate/logs" hx-target="#gate-logs-table" class="text-[10px] text-stone-600 hover:text-stone-900 underline">Refresh Logs</button>
                            </div>
                            <div id="gate-logs-table" hx-get="/api/v1/gate/logs" hx-trigger="load" class="max-h-40 overflow-y-auto">
                                <!-- Populated dynamically -->
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- TAB 3: WILD-PMS & INCIDENT COMMAND SYSTEM -->
            <div id="workspace-wild-pms" class="domain-workspace hidden space-y-6 max-w-5xl mx-auto">
                <div class="border-b border-[#e6dfd5] pb-3 flex items-center justify-between">
                    <div>
                        <h2 class="text-xl font-bold font-serif text-stone-900">Ranger Incident Command & Facilities Maintenance</h2>
                        <p class="text-xs text-stone-600">Park ranger incident dispatch, campsite repair work orders, and real-time campground headcount.</p>
                    </div>
                    <span class="px-2 py-1 bg-amber-100 text-amber-900 rounded font-mono text-xs font-bold">Ranger Safety & Dispatch</span>
                </div>

                <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
                    <!-- Left: Incident Reporting Form -->
                    <div class="sepia-card border rounded p-4 space-y-3 shadow-sm">
                        <div class="font-bold text-sm text-stone-900">Report Wilderness Incident</div>
                        <form hx-post="/api/v1/incidents" hx-target="#ics-active-incidents" class="space-y-3 text-xs">
                            <div>
                                <label class="block font-medium text-stone-700 mb-1">Incident Type</label>
                                <select name="type" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                    <option value="BEAR_SIGHTING">BEAR_SIGHTING (Grizzly / Black Bear)</option>
                                    <option value="FOOD_CONDITIONED_ANIMAL">FOOD_CONDITIONED_ANIMAL (Aggressive habituation)</option>
                                    <option value="TRAIL_WASHOUT">TRAIL_WASHOUT (Pass impassable)</option>
                                    <option value="SEARCH_AND_RESCUE">SEARCH_AND_RESCUE (Overdue hiker)</option>
                                    <option value="SMOKE_HAZARD">SMOKE_HAZARD (Uncontrolled fire line)</option>
                                </select>
                            </div>
                            <div>
                                <label class="block font-medium text-stone-700 mb-1">Severity Rating</label>
                                <select name="severity" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                    <option value="ADVISORY">ADVISORY (Level 1: Informational)</option>
                                    <option value="TRAIL_CLOSURE">TRAIL_CLOSURE (Level 2: Sector closed)</option>
                                    <option value="EVACUATION_WARNING">EVACUATION_WARNING (Level 3: Prepare)</option>
                                    <option value="IMMEDIATE_EVACUATION">IMMEDIATE_EVACUATION (Level 4: Life safety)</option>
                                </select>
                            </div>
                            <div>
                                <label class="block font-medium text-stone-700 mb-1">Sector / Trailhead Location</label>
                                <input type="text" name="sector" value="Upper Loop Sector B" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3]">
                            </div>
                            <div>
                                <label class="block font-medium text-stone-700 mb-1">GPS Coordinates</label>
                                <input type="text" name="gps_trailhead" value="46.8523,-121.7603" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                            </div>
                            <div>
                                <label class="block font-medium text-stone-700 mb-1">Description & Field Observations</label>
                                <textarea name="description" rows="2" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-serif">Adult black bear investigating food storage locker near site B-14.</textarea>
                            </div>
                            <button type="submit" class="w-full sepia-btn-primary py-2 rounded text-xs font-medium">Broadcast Incident to Ranger Station</button>
                        </form>
                    </div>

                    <!-- Center: Active Incident Feed & Muster Roll -->
                    <div class="sepia-card border rounded p-4 space-y-3 shadow-sm flex flex-col justify-between">
                        <div class="space-y-3">
                            <div class="flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                                <span class="font-bold text-sm text-stone-900">Active Incident Feed</span>
                                <button hx-get="/api/v1/incidents/muster?campground_id=CAMP-PACIFIC-01" hx-target="#ics-muster-output" class="text-[10px] text-rose-700 font-bold hover:underline">
                                    Generate Muster Roll
                                </button>
                            </div>
                            <div id="ics-active-incidents" hx-get="/api/v1/incidents" hx-trigger="load" class="space-y-2">
                                <!-- Populated dynamically -->
                            </div>
                            <div id="ics-muster-output"></div>
                        </div>
                    </div>

                    <!-- Right: Campsite Asset Maintenance & Occupancy Headcount -->
                    <div class="space-y-4">
                        <!-- Asset Maintenance -->
                        <div class="sepia-card border rounded p-4 space-y-3 shadow-sm">
                            <div class="flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                                <span class="font-bold text-sm text-stone-900">Infrastructure Repair</span>
                                <span class="text-[10px] font-mono text-amber-800">Maintenance Log</span>
                            </div>
                            <div id="pms-damaged-assets" hx-get="/api/v1/pms/assets" hx-trigger="load" class="space-y-2">
                                <!-- Populated dynamically -->
                            </div>
                        </div>

                        <!-- Real-time In-Park Headcount Roster -->
                        <div class="sepia-card border rounded p-4 space-y-3 shadow-sm">
                            <div class="flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                                <span class="font-bold text-sm text-stone-900">In-Park Occupancy</span>
                                <span class="text-[10px] font-mono text-emerald-800">Current Campers</span>
                            </div>
                            <div id="pms-occupancy-roster" hx-get="/api/v1/pms/occupancy" hx-trigger="load" class="space-y-2">
                                <!-- Populated dynamically -->
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- TAB 4: BACKCOUNTRY QUOTAS & OUTFITTER LOCKERS -->
            <div id="workspace-backcountry-lockers" class="domain-workspace hidden space-y-6 max-w-5xl mx-auto">
                <div class="border-b border-[#e6dfd5] pb-3 flex items-center justify-between">
                    <div>
                        <h2 class="text-xl font-bold font-serif text-stone-900">Backcountry Permits & Outfitter Gear Lockers</h2>
                        <p class="text-xs text-stone-600">Fair lottery draws for backcountry trailheads and 24/7 contactless gear rental lockers.</p>
                    </div>
                    <span class="px-2 py-1 bg-blue-100 text-blue-900 rounded font-mono text-xs font-bold">Fair Permit Draw & Gear Lockers</span>
                </div>

                <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
                    <!-- Left: Trailhead Quotas & Provably Fair Lottery -->
                    <div class="sepia-card border rounded p-4 space-y-4 shadow-sm">
                        <div class="flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                            <span class="font-bold text-sm text-stone-900">Trailhead Quota Tracker</span>
                            <span class="text-[10px] font-mono text-stone-500">Alpine Zones</span>
                        </div>
                        <div id="trailhead-zones-container" hx-get="/api/v1/backcountry/zones" hx-trigger="load" class="space-y-2">
                            <!-- Populated dynamically -->
                        </div>

                        <div id="lottery-application-status"></div>

                        <!-- Provably Fair Draw Execution Box -->
                        <div class="p-3 bg-stone-50 border border-stone-200 rounded space-y-3 text-xs">
                            <div class="font-bold text-stone-800 flex items-center justify-between">
                                <span>Transparent Trailhead Permit Lottery</span>
                                <span class="text-[9px] bg-stone-200 px-1 py-0.5 rounded font-mono">Fair Permit Algorithm</span>
                            </div>
                            <p class="text-[11px] text-stone-600">Prior to the permit draw, an immutable seed is published so all permit applicants receive impartial lottery selection.</p>
                            <div class="grid grid-cols-2 gap-2">
                                <button hx-post="/api/v1/backcountry/commitment" hx-vals='{"trailhead_id":"zone-enchantments","secret_salt":"alpine-salt-2026"}' hx-target="#lottery-commitment-output" class="py-1.5 bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-800 rounded font-mono text-xs border border-[#ded5c7]">
                                    Lock Permit Seed
                                </button>
                                <button hx-post="/api/v1/backcountry/draw" hx-vals='{"trailhead_id":"zone-enchantments","secret_salt":"alpine-salt-2026"}' hx-target="#lottery-draw-results" class="py-1.5 sepia-btn-primary rounded font-mono text-xs">
                                    Draw Winning Applicants
                                </button>
                            </div>
                            <div id="lottery-commitment-output"></div>
                            <div id="lottery-draw-results"></div>
                        </div>
                    </div>

                    <!-- Right: Outfitter Gear & 16-Bay Smart Lockers -->
                    <div class="sepia-card border rounded p-4 space-y-4 shadow-sm flex flex-col justify-between">
                        <div class="space-y-4">
                            <div class="flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                                <div>
                                    <span class="font-bold text-sm text-stone-900">Wilderness Outfitter Gear Catalog</span>
                                    <div class="text-[10px] text-stone-500">Bear Canisters · Satellite Messengers · 4-Season Tents</div>
                                </div>
                                <span class="px-2 py-0.5 bg-emerald-100 text-emerald-800 text-[10px] font-mono rounded font-bold">16-Bay Locker Bank</span>
                            </div>

                            <div id="gear-catalog-container" hx-get="/api/v1/outfitter/gear" hx-trigger="load" class="space-y-2">
                                <!-- Populated dynamically -->
                            </div>

                            <div id="locker-provision-status"></div>

                            <!-- 16-Bay Locker Bank Status Grid -->
                            <div class="border-t border-[#e6dfd5] pt-3 space-y-2">
                                <div class="flex items-center justify-between text-xs font-bold text-stone-800">
                                    <span>Locker Terminal Status (Bays 1–16)</span>
                                    <button hx-get="/api/v1/outfitter/lockers" hx-target="#locker-grid-container" class="text-[10px] text-stone-500 hover:underline">Refresh</button>
                                </div>
                                <div id="locker-grid-container" hx-get="/api/v1/outfitter/lockers" hx-trigger="load">
                                    <!-- Populated dynamically -->
                                </div>
                            </div>

                            <!-- Contactless Solenoid Unlatch Terminal -->
                            <form hx-post="/api/v1/outfitter/unlock" hx-target="#locker-unlock-output" class="p-3 bg-[#fbf8f3] border border-[#e6dfd5] rounded space-y-2 text-xs">
                                <div class="font-bold text-stone-800 text-[11px]">Gear Locker Kiosk Terminal</div>
                                <div class="grid grid-cols-2 gap-2">
                                    <input type="number" name="bay_number" placeholder="Bay # (e.g. 1)" required class="p-1.5 border border-[#e6dfd5] rounded bg-white font-mono">
                                    <input type="text" name="pin" placeholder="6-Digit OTP PIN" required maxlength="6" class="p-1.5 border border-[#e6dfd5] rounded bg-white font-mono tracking-widest font-bold">
                                </div>
                                <button type="submit" class="w-full sepia-btn-forest py-1.5 rounded text-xs font-mono font-medium">Unlock Locker Door</button>
                                <div id="locker-unlock-output"></div>
                            </form>
                        </div>
                    </div>
                </div>
            </div>

            <!-- TAB 5: WILDFIRE PHYSICS & VERTEX AI GEMINI BRIEFING -->
            <div id="workspace-rothermel-telemetry" class="domain-workspace hidden space-y-6 max-w-5xl mx-auto">
                <div class="border-b border-[#e6dfd5] pb-3 flex items-center justify-between">
                    <div>
                        <h2 class="text-xl font-bold font-serif text-stone-900">Wildfire Safety & Ranger Conditions Briefing</h2>
                        <p class="text-xs text-stone-600">Wildland fire spread modeling, elevation weather sensor mesh, and tactical safety advisories.</p>
                    </div>
                    <span class="px-2 py-1 bg-amber-100 text-amber-900 rounded font-mono text-xs font-bold">Wildfire Safety & Weather Mesh</span>
                </div>

                <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
                    <!-- Left: Rothermel Fire Behavior Physics Engine -->
                    <div class="sepia-card border rounded p-4 space-y-4 shadow-sm">
                        <div class="font-bold text-sm text-stone-900 flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                            <span>Rothermel Surface Spread Calculator</span>
                            <span class="text-[10px] font-mono text-stone-400">Physics Core</span>
                        </div>
                        <form hx-post="/api/v1/telemetry/rothermel" hx-target="#rothermel-calc-output" class="space-y-3 text-xs">
                            <div>
                                <label class="block font-medium text-stone-700 mb-1">Standard Fire Behavior Fuel Model</label>
                                <select name="fuel_model_number" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                    <option value="1">Model 1: Short Grass (1 ft deep, rapid spread, low flame)</option>
                                    <option value="4">Model 4: Chaparral / High Shrub (6 ft deep, high intensity)</option>
                                    <option value="8">Model 8: Closed Timber Litter (Compact needle cast, slow)</option>
                                    <option value="10">Model 10: Heavy Timber / Downed Conifer Understory</option>
                                </select>
                            </div>
                            <div class="grid grid-cols-3 gap-2">
                                <div>
                                    <label class="block font-medium text-stone-700 mb-1">Wind (mph)</label>
                                    <input type="number" step="0.5" name="wind_speed_mph" value="14.0" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                </div>
                                <div>
                                    <label class="block font-medium text-stone-700 mb-1">Slope (°)</label>
                                    <input type="number" step="1" name="slope_deg" value="18.0" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                </div>
                                <div>
                                    <label class="block font-medium text-stone-700 mb-1">Fuel Moisture (%)</label>
                                    <input type="number" step="0.5" name="fuel_moisture_pct" value="7.5" class="w-full p-2 border border-[#e6dfd5] rounded bg-[#fbf8f3] font-mono">
                                </div>
                            </div>
                            <button type="submit" class="w-full sepia-btn-primary py-2 rounded text-xs font-mono font-medium">
                                Calculate Fire Spread & Flame Intensity
                            </button>
                        </form>

                        <div id="rothermel-calc-output">
                            <!-- Populated on calculation -->
                        </div>

                        <!-- Elevation Sensor Mesh -->
                        <div class="border-t border-[#e6dfd5] pt-3 space-y-2">
                            <div class="flex items-center justify-between text-xs font-bold text-stone-800">
                                <span>Backcountry Elevation Sensor Mesh</span>
                                <button hx-get="/api/v1/telemetry/mesh" hx-target="#sensor-mesh-container" class="text-[10px] text-stone-500 hover:underline">Refresh Mesh</button>
                            </div>
                            <div id="sensor-mesh-container" hx-get="/api/v1/telemetry/mesh" hx-trigger="load" class="space-y-2">
                                <!-- Populated dynamically -->
                            </div>
                        </div>
                    </div>

                    <!-- Right: Gemini 3.8 Flash Tactical Briefing & Offline Sync -->
                    <div class="sepia-card border rounded p-4 space-y-4 shadow-sm flex flex-col justify-between">
                        <div class="space-y-4">
                            <div class="flex items-center justify-between pb-2 border-b border-[#e6dfd5]">
                                <div>
                                    <span class="font-bold text-sm text-stone-900">Ranger AI Safety Briefing</span>
                                    <div class="text-[10px] text-stone-500">Automated Wilderness Advisory</div>
                                </div>
                                <span class="px-2 py-0.5 bg-blue-100 text-blue-900 text-[10px] font-mono rounded font-bold">Active Park Dispatch</span>
                            </div>

                            <form hx-post="/api/v1/telemetry/briefing" hx-target="#tactical-briefing-output" class="space-y-2">
                                <p class="text-xs text-stone-600 leading-tight">Synthesize real-time weather observations, active ICS incidents, and gatehouse queues into an actionable incident action plan.</p>
                                <button type="submit" class="w-full sepia-btn-forest py-2 rounded text-xs font-mono font-medium flex items-center justify-center space-x-1">
                                    <span>🤖</span><span>Generate Ranger Safety Briefing</span>
                                </button>
                            </form>

                            <div id="tactical-briefing-output">
                                <!-- Populated dynamically -->
                            </div>

                            <!-- Offline Gate Cache & WAL Inspector -->
                            <div class="border-t border-[#e6dfd5] pt-3 space-y-2 text-xs">
                                <div class="flex items-center justify-between font-bold text-stone-800">
                                    <span>Remote Gatehouse Offline Terminal</span>
                                    <button hx-get="/api/v1/sync/wal" hx-target="#offline-wal-container" class="text-[10px] text-stone-500 hover:underline">View Offline Queue</button>
                                </div>
                                <p class="text-[11px] text-stone-600">Allows park staff to check in arriving campers during mountain satellite network disconnections.</p>
                                
                                <form hx-post="/api/v1/sync/authorize-offline" hx-target="#offline-wal-container" class="flex space-x-2">
                                    <input type="text" name="plate" value="WA-ALPINE1" placeholder="Plate" class="p-1.5 border border-[#e6dfd5] rounded bg-[#fbf8f3] text-xs font-mono w-1/2">
                                    <input type="text" name="state" value="WA" class="p-1.5 border border-[#e6dfd5] rounded bg-[#fbf8f3] text-xs font-mono w-1/4">
                                    <button type="submit" class="px-3 py-1.5 bg-stone-800 text-white rounded text-xs font-mono font-medium w-1/4">Check In Camper (Offline Mode)</button>
                                </form>

                                <div id="offline-wal-container" hx-get="/api/v1/sync/wal" hx-trigger="load">
                                    <!-- Populated dynamically -->
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </main>

        <!-- Right Panel: Contextual Drawer & Live SSE Stream -->
        <aside class="w-80 border-l border-[#e6dfd5] bg-[#f4efe6] p-4 flex flex-col justify-between overflow-y-auto flex-shrink-0" id="calendar-drawer">
            <div class="space-y-4">
                <div class="border-b border-[#e6dfd5] pb-2 flex items-center justify-between">
                    <span class="font-bold text-xs font-serif text-stone-900">Park Status & Live Updates</span>
                    <span class="px-1.5 py-0.5 bg-emerald-100 text-emerald-800 text-[9px] font-mono rounded">SSE STREAMING</span>
                </div>

                <!-- Live SSE Event Stream Box -->
                <div class="p-3 bg-white border border-[#e6dfd5] rounded shadow-sm space-y-2">
                    <div class="font-bold text-xs text-stone-800 flex items-center justify-between">
                        <span>Campground Activity Stream</span>
                        <span class="text-[9px] text-emerald-600 font-mono">● LIVE</span>
                    </div>
                    <div id="sse-event-log" class="text-[10px] font-mono space-y-1 max-h-48 overflow-y-auto divide-y divide-stone-100">
                        <div class="text-stone-400 py-0.5">Connected to /api/v1/ranger/events</div>
                        <div class="text-stone-600 py-0.5">GATE-AUTH: WA-ALPINE1 granted (Standard Lane)</div>
                        <div class="text-stone-600 py-0.5">PMS: Bear box B-14 inspected</div>
                    </div>
                </div>

                <!-- Live NOAA Microclimate Feed -->
                <div class="p-3 bg-white border border-[#e6dfd5] rounded shadow-sm space-y-2 text-xs">
                    <div class="font-bold text-stone-800 flex items-center justify-between">
                        <span>Rainier Weather Advisory</span>
                        <span class="text-[9px] font-mono text-stone-400">NOAA API</span>
                    </div>
                    <div id="sidebar-weather-feed" hx-get="/api/v1/weather/microclimate?lat=46.8523&lon=-121.7603" hx-trigger="load">
                        <div class="text-stone-400 text-[11px]">Loading live NOAA telemetry...</div>
                    </div>
                </div>

                <!-- Wilderness Guidelines Card -->
                <div class="p-3 bg-white border border-[#e6dfd5] rounded shadow-sm space-y-1.5 text-xs">
                    <div class="font-bold text-stone-900 font-serif">Wilderness Guidelines</div>
                    <div class="text-[10px] text-stone-600">Store food in bear-resistant canisters.</div>
                    <div class="text-[10px] text-stone-600">Pack out all trash and waste.</div>
                    <div class="text-[10px] text-emerald-700 font-bold font-mono">Leave No Trace Verified 🌲</div>
                </div>
            </div>
        </aside>
    </div>

    <!-- Auth Modal -->
    <div id="auth-modal" class="hidden fixed inset-0 bg-stone-900/40 backdrop-blur-sm flex items-center justify-center z-50">
        <div class="bg-white rounded-lg shadow-xl w-96 p-6 border border-stone-200">
            <div class="flex justify-between items-center mb-4">
                <h3 class="text-lg font-bold text-stone-800 font-serif">Sign In or Register</h3>
                <button onclick="document.getElementById('auth-modal').classList.add('hidden')" class="text-stone-400 hover:text-stone-600">✕</button>
            </div>
            
            <form hx-post="/login" hx-target="#auth-error" class="space-y-3">
                <input type="email" name="email" placeholder="email@camping.local" required class="w-full text-xs px-3 py-2 border rounded border-stone-300">
                <input type="password" name="password" placeholder="Password" required class="w-full text-xs px-3 py-2 border rounded border-stone-300">
                <button type="submit" class="w-full sepia-btn-primary py-2 rounded text-xs font-medium">Sign In</button>
            </form>
            
            <div class="relative my-4">
                <div class="absolute inset-0 flex items-center"><div class="w-full border-t border-stone-200"></div></div>
                <div class="relative flex justify-center text-xs"><span class="bg-white px-2 text-stone-400 font-mono text-[10px]">NEW CAMPER ACCOUNT</span></div>
            </div>

            <form hx-post="/signup" hx-target="#auth-error" class="space-y-3">
                <input type="text" name="full_name" placeholder="Full Legal Name" required class="w-full text-xs px-3 py-2 border rounded border-stone-300">
                <input type="email" name="email" placeholder="Camper Email" required class="w-full text-xs px-3 py-2 border rounded border-stone-300">
                <input type="password" name="password" placeholder="Password" required class="w-full text-xs px-3 py-2 border rounded border-stone-300">
                <button type="submit" class="w-full bg-[#eee8dc] hover:bg-[#e2dacf] text-stone-900 py-2 rounded text-xs font-medium">Create Account</button>
            </form>
            <div id="auth-error" class="mt-3"></div>
        </div>
    </div>

    <!-- Client-Side Tab Switching & SSE Listener -->
    <script>
        function switchTab(tabName) {
            document.querySelectorAll('.domain-workspace').forEach(el => el.classList.add('hidden'));
            const target = document.getElementById('workspace-' + tabName);
            if (target) {
                target.classList.remove('hidden');
            }

            // Update nav styles
            const navIds = ['campsites', 'fleet-gate', 'wild-pms', 'backcountry-lockers', 'rothermel-telemetry', 'bookings'];
            navIds.forEach(id => {
                const navBtn = document.getElementById('nav-' + id);
                if (navBtn) {
                    if (id === tabName) {
                        navBtn.className = 'w-full text-left px-3 py-2 rounded font-semibold bg-[#ded7c7] text-stone-900 flex items-center space-x-2';
                    } else {
                        navBtn.className = 'w-full text-left px-3 py-2 rounded font-medium hover:bg-[#ded7c7] text-stone-700 flex items-center space-x-2';
                    }
                }
            });
        }

        // Live SSE connection for Wilderness Event Bus
        const evtSource = new EventSource('/api/v1/ranger/events');
        evtSource.onmessage = function(e) {
            const logContainer = document.getElementById('sse-event-log');
            if (logContainer) {
                const line = document.createElement('div');
                line.className = 'py-0.5 text-stone-700 border-t border-stone-100';
                line.textContent = e.data;
                logContainer.insertBefore(line, logContainer.firstChild);
            }
        };
    </script>
</body>
</html>
`

	cardsHTML := `
{{define "campsite-cards"}}
<div class="max-w-5xl mx-auto space-y-4">
    <div class="flex justify-between items-center mb-2">
        <h2 class="text-xl font-bold font-serif text-stone-900">Available Campgrounds ({{len .Campsites}})</h2>
        <span class="text-xs font-mono text-stone-500">Real-Time Site Availability</span>
    </div>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-5">
        {{range .Campsites}}
        <div class="sepia-card border rounded-lg overflow-hidden shadow-sm hover:shadow transition flex flex-col justify-between">
            <div>
                <img src="{{.ImageURL}}" alt="{{.Name}}" class="w-full h-44 object-cover">
                <div class="p-4 space-y-2">
                    <div class="flex justify-between items-start">
                        <div>
                            <span class="text-[10px] uppercase font-mono tracking-wider font-semibold px-2 py-0.5 rounded bg-stone-100 text-stone-600">{{.CampsiteType}}</span>
                            <h3 class="text-base font-bold text-stone-900 mt-1 font-serif">{{.Name}}</h3>
                            <p class="text-xs text-stone-500">📍 {{.Location}}</p>
                        </div>
                        <div class="text-right">
                            <span class="text-lg font-bold text-stone-900 font-mono">${{printf "%.2f" (div .DailyRateCents 100)}}</span>
                            <span class="text-xs text-stone-500 block">/ night</span>
                        </div>
                    </div>
                    <p class="text-xs text-stone-600 line-clamp-2">{{.Description}}</p>
                    <div class="flex flex-wrap gap-1 pt-1">
                        {{range .Amenities}}
                        <span class="text-[10px] px-1.5 py-0.5 rounded bg-[#f4efe6] text-stone-600">{{.}}</span>
                        {{end}}
                    </div>
                </div>
            </div>
            <div class="p-4 pt-0">
                <button hx-get="/availability?campsite_id={{.ID}}" hx-target="#calendar-drawer" class="w-full sepia-btn-primary py-2 rounded text-xs font-medium font-mono">
                    Check Live Availability & Reserve
                </button>
            </div>
        </div>
        {{end}}
    </div>
</div>
{{end}}
`

	calendarHTML := `
{{define "availability-calendar"}}
<div class="space-y-4 font-sans">
    <div class="border-b border-[#e6dfd5] pb-3">
        <span class="text-[10px] uppercase font-mono tracking-wider px-2 py-0.5 rounded bg-stone-200 text-stone-700">{{.Campsite.CampsiteType}}</span>
        <h3 class="text-lg font-bold text-stone-900 mt-1 font-serif">{{.Campsite.Name}}</h3>
        <p class="text-xs text-stone-500">📍 {{.Campsite.Location}}</p>
        <p class="text-sm font-bold text-stone-900 mt-1 font-mono">${{printf "%.2f" (div .Campsite.DailyRateCents 100)}} <span class="text-xs font-normal text-stone-500">/ night</span></p>
    </div>

    <!-- Active Hold Status -->
    {{if .Holds}}
    <div class="p-2.5 bg-amber-50 border border-amber-300 rounded text-xs font-mono space-y-1">
        <div class="font-bold text-amber-900 flex items-center justify-between">
            <span>⚡ Active Reservation Holds ({{len .Holds}})</span>
            <span class="text-[10px] bg-amber-200 px-1 py-0.5 rounded">15m Hold</span>
        </div>
        {{range .Holds}}
        <div class="text-[11px] text-amber-800">
            Token: <span class="font-bold">{{.TokenID}}</span> · Exp: {{.ExpiresAt.Format "15:04:05"}}
        </div>
        {{end}}
    </div>
    {{end}}

    <!-- Reservation Booking Form -->
    <form hx-post="/bookings" hx-target="#booking-result" class="space-y-3">
        <input type="hidden" name="campsite_id" value="{{.Campsite.ID}}">
        <div class="grid grid-cols-2 gap-2 text-xs">
            <div>
                <label class="block font-medium text-stone-600 mb-1">Check-in Date</label>
                <input type="date" name="start_date" id="book-start" required class="w-full text-xs p-2 rounded border border-[#e6dfd5] bg-white">
            </div>
            <div>
                <label class="block font-medium text-stone-600 mb-1">Check-out Date</label>
                <input type="date" name="end_date" id="book-end" required class="w-full text-xs p-2 rounded border border-[#e6dfd5] bg-white">
            </div>
        </div>
        <div>
            <label class="block text-xs font-medium text-stone-600 mb-1">Number of Guests (Max {{.Campsite.Capacity}})</label>
            <input type="number" name="guests_count" value="2" min="1" max="{{.Campsite.Capacity}}" class="w-full text-xs p-2 rounded border border-[#e6dfd5] bg-white">
        </div>
        <button type="submit" class="w-full sepia-btn-primary py-2.5 rounded text-xs font-medium">
            Confirm Campsite Reservation
        </button>
    </form>
    <div id="booking-result" class="mt-2"></div>
</div>
{{end}}
`

	bookingsHTML := `
{{define "my-bookings"}}
<div class="max-w-4xl mx-auto space-y-4">
    <div class="border-b border-[#e6dfd5] pb-3 flex justify-between items-center">
        <div>
            <h2 class="text-xl font-bold font-serif text-stone-900">My Stays & Gate Permits</h2>
            <p class="text-xs text-stone-500">Confirmed park permits & campsite reservations</p>
        </div>
        <button hx-get="/" hx-target="#campsite-stream" onclick="switchTab('campsites')" class="text-xs sepia-btn-primary px-3 py-1.5 rounded">
            Explore More Sites
        </button>
    </div>

    {{if not .Bookings}}
        <div class="text-center py-12 text-stone-400 text-sm">
            You do not have any active bookings yet.
        </div>
    {{else}}
        <div class="space-y-3">
            {{range .Bookings}}
            <div class="p-4 bg-white border border-[#e6dfd5] rounded-lg shadow-sm flex items-center justify-between">
                <div>
                    <div class="flex items-center space-x-2">
                        <span class="font-bold text-stone-900 font-serif">{{.CampsiteName}}</span>
                        <span class="text-[10px] px-2 py-0.5 rounded font-mono uppercase {{if eq .Status "confirmed"}}bg-emerald-100 text-emerald-800{{else}}bg-stone-100 text-stone-500{{end}}">
                            {{.Status}}
                        </span>
                    </div>
                    <p class="text-xs text-stone-600 mt-1">📅 {{.StartDate.Format "Jan 02, 2006"}} – {{.EndDate.Format "Jan 02, 2006"}} · Total: ${{printf "%.2f" (div .TotalCents 100)}}</p>
                    <p class="text-[10px] text-stone-400 font-mono mt-0.5">Booking Ref: {{.ID}}</p>
                </div>
                <div>
                    {{if eq .Status "confirmed"}}
                    <button hx-post="/cancel-booking" hx-vals='{"booking_id": "{{.ID}}"}' hx-target="#campsite-stream" hx-confirm="Are you sure you want to cancel this reservation?" class="text-xs text-rose-700 hover:text-rose-900 font-medium px-3 py-1.5 border border-rose-200 rounded hover:bg-rose-50 font-mono">
                        Cancel Booking
                    </button>
                    {{else}}
                    <span class="text-xs text-stone-400 italic">Cancelled</span>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>
    {{end}}
</div>
{{end}}
`

	simHTML := `
{{define "simulation-results"}}
<div class="p-2.5 bg-white border border-stone-300 rounded text-[11px] space-y-1.5 font-mono">
    <div class="font-bold text-stone-800 text-xs">Simultaneous Booking Attempt</div>
    <div class="text-[10px] text-stone-500">Dates: {{.DateRange}}</div>
    {{range .Results}}
    <div class="flex items-center justify-between border-t border-stone-100 pt-1">
        <span>{{.User}}</span>
        <span class="{{if .Success}}text-emerald-700 font-bold{{else}}text-rose-600 font-bold{{end}}">
            {{if .Success}}✅ BOOKING CONFIRMED{{else}}❌ SITE ALREADY HELD{{end}}
        </span>
    </div>
    <div class="text-[10px] text-stone-400">{{.Message}} ({{.Latency}})</div>
    {{end}}
</div>
{{end}}
`

	tmpl := template.New("index.html").Funcs(template.FuncMap{
		"div": func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b)
		},
	})

	template.Must(tmpl.Parse(layoutHTML))
	template.Must(tmpl.Parse(cardsHTML))
	template.Must(tmpl.Parse(calendarHTML))
	template.Must(tmpl.Parse(bookingsHTML))
	template.Must(tmpl.Parse(simHTML))

	s.tmpl = tmpl
}

