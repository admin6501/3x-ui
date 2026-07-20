#!/usr/bin/env python3
"""
Test script for 3x-ui panel remarkTemplate bug fix
Tests that empty/whitespace remarkTemplate resets to default, custom values preserved
"""

import requests
import json
import re
from typing import Dict, Optional

BASE_URL = "http://localhost:2053"
# Note: The actual default in the code uses double braces {{}} not single braces {}
DEFAULT_REMARK_TEMPLATE = "{{EMAIL}}|{{INBOUND}}|📊{{TRAFFIC_LEFT}}|⏳{{DAYS_LEFT}}D"

class PanelTester:
    def __init__(self):
        self.session = requests.Session()
        self.csrf_token = None
        
    def extract_csrf_from_html(self, html: str) -> Optional[str]:
        """Extract CSRF token from login page HTML"""
        match = re.search(r'<meta name="csrf-token" content="([^"]+)"', html)
        return match.group(1) if match else None
    
    def login(self, username: str, password: str) -> bool:
        """Perform login with CSRF token handling"""
        print(f"\n[AUTH] Step 1: Getting initial CSRF token from login page...")
        
        # Step 1: GET login page to get initial CSRF token and session cookie
        resp = self.session.get(f"{BASE_URL}/")
        if resp.status_code != 200:
            print(f"❌ Failed to get login page: {resp.status_code}")
            return False
        
        initial_csrf = self.extract_csrf_from_html(resp.text)
        if not initial_csrf:
            print(f"❌ Could not extract CSRF token from login page")
            return False
        
        print(f"✅ Got initial CSRF token: {initial_csrf[:20]}...")
        
        # Step 2: POST login with initial CSRF token
        print(f"\n[AUTH] Step 2: Logging in as {username}...")
        login_data = {
            "username": username,
            "password": password
        }
        headers = {
            "Content-Type": "application/json",
            "X-CSRF-Token": initial_csrf,
            "X-Requested-With": "XMLHttpRequest"
        }
        
        resp = self.session.post(f"{BASE_URL}/login", json=login_data, headers=headers)
        if resp.status_code != 200:
            print(f"❌ Login failed with status {resp.status_code}")
            print(f"Response: {resp.text}")
            return False
        
        try:
            result = resp.json()
            if not result.get("success"):
                print(f"❌ Login failed: {result}")
                return False
        except json.JSONDecodeError:
            print(f"❌ Invalid JSON response from login: {resp.text}")
            return False
        
        print(f"✅ Login successful")
        
        # Step 3: Get fresh CSRF token for subsequent requests
        print(f"\n[AUTH] Step 3: Getting fresh CSRF token for API calls...")
        resp = self.session.get(f"{BASE_URL}/panel/csrf-token")
        if resp.status_code != 200:
            print(f"❌ Failed to get fresh CSRF token: {resp.status_code}")
            return False
        
        try:
            result = resp.json()
            if not result.get("success"):
                print(f"❌ CSRF token fetch failed: {result}")
                return False
            
            self.csrf_token = result.get("obj")
            if not self.csrf_token:
                print(f"❌ No CSRF token in response: {result}")
                return False
            
            print(f"✅ Got fresh CSRF token: {self.csrf_token[:20]}...")
            return True
            
        except json.JSONDecodeError:
            print(f"❌ Invalid JSON response from csrf-token: {resp.text}")
            return False
    
    def get_all_settings(self) -> Optional[Dict]:
        """Fetch all settings from the panel"""
        headers = {
            "Content-Type": "application/json",
            "X-CSRF-Token": self.csrf_token,
            "X-Requested-With": "XMLHttpRequest"
        }
        
        resp = self.session.post(f"{BASE_URL}/panel/api/setting/all", headers=headers)
        if resp.status_code != 200:
            print(f"❌ Failed to get settings: {resp.status_code}")
            print(f"Response: {resp.text}")
            return None
        
        try:
            result = resp.json()
            if not result.get("success"):
                print(f"❌ Get settings failed: {result}")
                return None
            
            return result.get("obj")
        except json.JSONDecodeError:
            print(f"❌ Invalid JSON response: {resp.text}")
            return None
    
    def update_settings(self, settings: Dict) -> bool:
        """Update all settings"""
        headers = {
            "Content-Type": "application/json",
            "X-CSRF-Token": self.csrf_token,
            "X-Requested-With": "XMLHttpRequest"
        }
        
        resp = self.session.post(f"{BASE_URL}/panel/api/setting/update", json=settings, headers=headers)
        if resp.status_code != 200:
            print(f"❌ Failed to update settings: {resp.status_code}")
            print(f"Response: {resp.text}")
            return False
        
        try:
            result = resp.json()
            if not result.get("success"):
                print(f"❌ Update settings failed: {result}")
                return False
            
            return True
        except json.JSONDecodeError:
            print(f"❌ Invalid JSON response: {resp.text}")
            return False
    
    def test_remark_template(self):
        """Run all remarkTemplate tests"""
        print("\n" + "="*80)
        print("TESTING REMARK TEMPLATE BUG FIX")
        print("="*80)
        
        # Login
        if not self.login("admin", "Admin12345"):
            print("\n❌ OVERALL RESULT: FAILED - Could not authenticate")
            return False
        
        # Test A: Get current settings
        print("\n" + "-"*80)
        print("TEST A: Get current settings")
        print("-"*80)
        
        settings = self.get_all_settings()
        if not settings:
            print("❌ TEST A FAILED: Could not fetch settings")
            return False
        
        current_remark = settings.get("remarkTemplate", "")
        print(f"✅ TEST A PASSED: Current remarkTemplate = '{current_remark}'")
        
        # Test B: Empty string should reset to default
        print("\n" + "-"*80)
        print("TEST B: Empty string should reset to default")
        print("-"*80)
        
        settings["remarkTemplate"] = ""
        if not self.update_settings(settings):
            print("❌ TEST B FAILED: Could not update settings with empty remarkTemplate")
            return False
        
        settings = self.get_all_settings()
        if not settings:
            print("❌ TEST B FAILED: Could not fetch settings after update")
            return False
        
        new_remark = settings.get("remarkTemplate", "")
        if new_remark == DEFAULT_REMARK_TEMPLATE:
            print(f"✅ TEST B PASSED: Empty string reset to default")
            print(f"   Expected: '{DEFAULT_REMARK_TEMPLATE}'")
            print(f"   Got:      '{new_remark}'")
        else:
            print(f"❌ TEST B FAILED: Empty string did NOT reset to default")
            print(f"   Expected: '{DEFAULT_REMARK_TEMPLATE}'")
            print(f"   Got:      '{new_remark}'")
            return False
        
        # Test C: Whitespace should reset to default
        print("\n" + "-"*80)
        print("TEST C: Whitespace should reset to default")
        print("-"*80)
        
        settings["remarkTemplate"] = "   "
        if not self.update_settings(settings):
            print("❌ TEST C FAILED: Could not update settings with whitespace remarkTemplate")
            return False
        
        settings = self.get_all_settings()
        if not settings:
            print("❌ TEST C FAILED: Could not fetch settings after update")
            return False
        
        new_remark = settings.get("remarkTemplate", "")
        if new_remark == DEFAULT_REMARK_TEMPLATE:
            print(f"✅ TEST C PASSED: Whitespace reset to default")
            print(f"   Expected: '{DEFAULT_REMARK_TEMPLATE}'")
            print(f"   Got:      '{new_remark}'")
        else:
            print(f"❌ TEST C FAILED: Whitespace did NOT reset to default")
            print(f"   Expected: '{DEFAULT_REMARK_TEMPLATE}'")
            print(f"   Got:      '{new_remark}'")
            return False
        
        # Test D: Custom value should be preserved
        print("\n" + "-"*80)
        print("TEST D: Custom value should be preserved")
        print("-"*80)
        
        custom_value = "{EMAIL}-mycustom"
        settings["remarkTemplate"] = custom_value
        if not self.update_settings(settings):
            print("❌ TEST D FAILED: Could not update settings with custom remarkTemplate")
            return False
        
        settings = self.get_all_settings()
        if not settings:
            print("❌ TEST D FAILED: Could not fetch settings after update")
            return False
        
        new_remark = settings.get("remarkTemplate", "")
        if new_remark == custom_value:
            print(f"✅ TEST D PASSED: Custom value preserved")
            print(f"   Expected: '{custom_value}'")
            print(f"   Got:      '{new_remark}'")
        else:
            print(f"❌ TEST D FAILED: Custom value was NOT preserved")
            print(f"   Expected: '{custom_value}'")
            print(f"   Got:      '{new_remark}'")
            return False
        
        print("\n" + "="*80)
        print("✅ ALL TESTS PASSED")
        print("="*80)
        return True


if __name__ == "__main__":
    tester = PanelTester()
    success = tester.test_remark_template()
    exit(0 if success else 1)
