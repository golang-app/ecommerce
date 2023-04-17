import React from "react";

import Container from "@mui/material/Container";
import Typography from "@mui/material/Typography";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Checkbox from "@mui/material/Checkbox";
import FormGroup from "@mui/material/FormGroup";
import FormControlLabel from "@mui/material/FormControlLabel";
import styled from "@emotion/styled";

const StyledBoxElement = styled("div")({
    padding: "10px 0 10px 0"
});

export interface LoginFormProps {
    onLogin: (email: string, password: string, rememberMe: boolean) => void;
}

export function LoginForm(props: LoginFormProps) {
    const [rememberMe, setRememberMe] = React.useState(false);

    const [email, setEmail] = React.useState("");
    const [emailError, setEmailError] = React.useState("");

    const [password, setPassword] = React.useState("");
    const [passwordError, setPasswordError] = React.useState("");

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

    const onLogin = () => {
        if (email === "") {
            setEmailError("Username is required");
            return;
        }

        if (password === "") {
            setPasswordError("Password is required");
            return;
        }

        props.onLogin(email, password, rememberMe)
    }

    return (
        <Container maxWidth="sm">
            <Typography variant="h3">Sign in</Typography>

            <StyledBoxElement>
                <TextField error={emailError !== ""} helperText={emailError} id="outlined-basic" label="Username" variant="outlined" fullWidth value={email} onChange={handleOnUsernameChange}/>
            </StyledBoxElement>
            <StyledBoxElement>
                <TextField error={passwordError !== ""} helperText={passwordError} label="Password" variant="outlined" type="password" fullWidth value={password} onChange={handleOnPasswordChange} />
            </StyledBoxElement>

            <StyledBoxElement>
                <FormGroup>
                    <FormControlLabel control={<Checkbox checked={rememberMe}  onClick={() => {
                        setRememberMe(!rememberMe)
                    }}/>} label="Remember me" />
                </FormGroup>
            </StyledBoxElement>

            <StyledBoxElement>
            <Button variant="contained" fullWidth color="primary" onClick={onLogin}>Login</Button>
            </StyledBoxElement>
        </Container>
    );
}
