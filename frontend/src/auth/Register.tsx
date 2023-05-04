import React from "react";

import Container from "@mui/material/Container";
import Typography from "@mui/material/Typography";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import styled from "@emotion/styled";

const StyledBoxElement = styled("div")({
    padding: "10px 0 10px 0"
});

export interface RegisterFormProps {
    onSignUp: (username: string, password: string) => void;
}

export function RegisterForm(props: RegisterFormProps) {
    const [email, setEmail] = React.useState("");
    const [emailError, setEmailError] = React.useState("");

    const [password, setPassword] = React.useState("");
    const [passwordError, setPasswordError] = React.useState("");

    const [password2, setPassword2] = React.useState("");
    const [password2Error, setPassword2Error] = React.useState("");

    const handleOnUsernameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        if (e.target.value !== "") {
            setEmailError("");
        }

        setEmail(e.target.value);
    }

    const handleOnPasswordChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        if (e.target.value !== "") {
            setPasswordError("");
        }
        
        setPassword(e.target.value);
    }

    const handleOnPassword2Change = (e: React.ChangeEvent<HTMLInputElement>) => {
        if (e.target.value !== "") {
            setPassword2Error("");
        }
        
        setPassword2(e.target.value);
    }

    const onLogin = () => {
        if (email === "") {
            setEmailError("E-mail is required");
            return;
        }

        if (password === "") {
            setPasswordError("Password is required");
            return;
        }

        if (password2 === "") {
            setPassword2Error("Password confirmation is required");
            return;
        }

        if (password !== password2) {
            setPassword2Error("Passwords do not match");
            return;
        }

        props.onSignUp(email, password)
    }

    return (
        <Container maxWidth="sm">
            <Typography variant="h3">Sign up</Typography>

            <StyledBoxElement>
                <TextField error={emailError !== ""} helperText={emailError} id="outlined-basic" label="Username" variant="outlined" fullWidth value={email} onChange={handleOnUsernameChange}/>
            </StyledBoxElement>
            <StyledBoxElement>
                <TextField error={passwordError !== ""} helperText={passwordError} label="Password" variant="outlined" type="password" fullWidth value={password} onChange={handleOnPasswordChange} />
            </StyledBoxElement>

            <StyledBoxElement>
                <TextField error={password2Error !== ""} helperText={password2Error} label="Confirm password" variant="outlined" type="password" fullWidth value={password2} onChange={handleOnPassword2Change} />
            </StyledBoxElement>

            <StyledBoxElement>
            <Button variant="contained" fullWidth color="primary" onClick={onLogin}>Sign up</Button>
            </StyledBoxElement>
        </Container>
    );
}
