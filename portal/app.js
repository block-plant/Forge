const API_URL = ''; // Relative to hosting

function showView(viewName) {
    document.querySelectorAll('.auth-card').forEach(el => el.classList.remove('active'));
    document.getElementById(viewName + '-view').classList.add('active');
}

function showToast(msg) {
    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.textContent = msg;
    document.getElementById('toast-container').appendChild(toast);
    setTimeout(() => toast.remove(), 3000);
}

// Signup
document.getElementById('signup-btn').addEventListener('click', async () => {
    const email = document.getElementById('signup-email').value;
    const password = document.getElementById('signup-password').value;

    try {
        const res = await fetch(`${API_URL}/auth/signup`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, password })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Signup failed');

        document.getElementById('verify-email-display').textContent = email;
        showView('otp');
    } catch (err) {
        showToast(err.message);
    }
});

// OTP Verification
document.querySelectorAll('.otp-input').forEach((input, idx) => {
    input.addEventListener('input', (e) => {
        if (e.target.value && idx < 5) {
            document.querySelector(`.otp-input[data-index="${idx + 1}"]`).focus();
        }
    });
    input.addEventListener('keydown', (e) => {
        if (e.key === 'Backspace' && !e.target.value && idx > 0) {
            document.querySelector(`.otp-input[data-index="${idx - 1}"]`).focus();
        }
    });
});

document.getElementById('verify-btn').addEventListener('click', async () => {
    const email = document.getElementById('signup-email').value;
    const code = Array.from(document.querySelectorAll('.otp-input')).map(i => i.value).join('');

    try {
        const res = await fetch(`${API_URL}/auth/verify-email`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, code })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Verification failed');

        showToast('Account verified! You can now log in.');
        showView('login');
    } catch (err) {
        showToast(err.message);
    }
});

// Login
document.getElementById('login-btn').addEventListener('click', async () => {
    const email = document.getElementById('login-email').value;
    const password = document.getElementById('login-password').value;

    try {
        const res = await fetch(`${API_URL}/auth/signin`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, password })
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Login failed');

        showToast('Login successful!');
        localStorage.setItem('forge_token', data.tokens.access_token);
        // Redirect or show app
        alert('Logged in! Token: ' + data.tokens.access_token);
    } catch (err) {
        showToast(err.message);
    }
});

// Forgot Password
document.getElementById('forgot-btn').addEventListener('click', async () => {
    const email = document.getElementById('forgot-email').value;

    try {
        const res = await fetch(`${API_URL}/auth/forgot-password`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email })
        });
        if (!res.ok) throw new Error('Failed to send reset code');

        showToast('Reset code sent!');
        showView('reset');
    } catch (err) {
        showToast(err.message);
    }
});

// Reset Password
document.getElementById('reset-btn').addEventListener('click', async () => {
    const email = document.getElementById('forgot-email').value;
    const code = document.getElementById('reset-code').value;
    const new_password = document.getElementById('reset-password').value;

    try {
        const res = await fetch(`${API_URL}/auth/reset-password`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, code, new_password })
        });
        if (!res.ok) throw new Error('Password reset failed');

        showToast('Password updated! Please log in.');
        showView('login');
    } catch (err) {
        showToast(err.message);
    }
});
